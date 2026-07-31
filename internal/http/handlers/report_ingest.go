package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"worksentry/internal/mq"
)

const maxClientEventIDLength = 96

type clientReportIngestEvent struct {
	IngestID      string                 `json:"ingestId"`
	SourceEventID string                 `json:"sourceEventId,omitempty"`
	ClientEventID string                 `json:"clientEventId,omitempty"`
	EmployeeID    int64                  `json:"employeeId"`
	ReceivedAt    string                 `json:"receivedAt"`
	ProcessName   string                 `json:"processName"`
	WindowTitle   string                 `json:"windowTitle"`
	IdleSeconds   int32                  `json:"idleSeconds"`
	Status        string                 `json:"status"`
	ClientVersion string                 `json:"clientVersion"`
	IPAddress     string                 `json:"ipAddress"`
	ReportType    string                 `json:"reportType"`
	Reason        string                 `json:"reason,omitempty"`
	Checkout      *ClientCheckoutPayload `json:"checkout,omitempty"`
}

func generateIngestID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	), nil
}

func normalizeClientEventID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len(value) > maxClientEventIDLength {
		return "", fmt.Errorf("clientEventId 长度不能超过 %d", maxClientEventIDLength)
	}
	return value, nil
}

func buildSourceEventID(employeeID int64, clientEventID string) string {
	if strings.TrimSpace(clientEventID) == "" {
		return ""
	}
	return fmt.Sprintf("%d:%s", employeeID, clientEventID)
}

func eventStatKey(event clientReportIngestEvent) string {
	if strings.TrimSpace(event.SourceEventID) != "" {
		return event.SourceEventID
	}
	return event.IngestID
}

func (h *Handler) acceptClientReportEvent(ctx context.Context, event clientReportIngestEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	outboxErr := h.insertClientReportOutbox(ctx, event, payload)
	mqErr := h.publishClientReportEvent(ctx, event.IngestID, payload)

	if mqErr == nil {
		_ = h.markClientReportOutboxPublished(ctx, event.IngestID, time.Now())
		return nil
	}

	if outboxErr == nil {
		_ = h.markClientReportOutboxFailed(ctx, event.IngestID, mqErr)
		return nil
	}

	return fmt.Errorf("事件未能写入 MQ 或 outbox: mq=%v; outbox=%v", mqErr, outboxErr)
}

func (h *Handler) beginClientReportProcessing(ctx context.Context, event clientReportIngestEvent) (bool, error) {
	if h.DB == nil {
		return false, errors.New("idempotency requires *sql.DB")
	}
	result, err := h.DB.ExecContext(ctx, `INSERT IGNORE INTO processed_ingests (ingest_id) VALUES (?)`, event.IngestID)
	if err != nil {
		return false, err
	}
	if rows, err := result.RowsAffected(); err == nil && rows == 0 {
		return true, nil
	}

	if strings.TrimSpace(event.SourceEventID) == "" {
		return false, nil
	}
	result, err = h.DB.ExecContext(ctx, `INSERT IGNORE INTO processed_source_events (
  source_event_id, first_ingest_id, employee_id, client_event_id
) VALUES (?, ?, ?, ?)`,
		event.SourceEventID,
		event.IngestID,
		event.EmployeeID,
		event.ClientEventID,
	)
	if err != nil {
		return false, err
	}
	if rows, err := result.RowsAffected(); err == nil && rows == 0 {
		return true, nil
	}
	return false, nil
}

func (h *Handler) completeClientReportProcessing(ctx context.Context, event clientReportIngestEvent) error {
	return nil
}

func (h *Handler) abandonClientReportProcessing(ctx context.Context, event clientReportIngestEvent) {
	if h.DB == nil || strings.TrimSpace(event.SourceEventID) == "" {
		return
	}
	_, _ = h.DB.ExecContext(ctx, `DELETE FROM processed_source_events
WHERE source_event_id = ? AND first_ingest_id = ?`, event.SourceEventID, event.IngestID)
}

func (h *Handler) insertClientReportOutbox(ctx context.Context, event clientReportIngestEvent, payload []byte) error {
	if h.DB == nil {
		return errors.New("outbox requires *sql.DB")
	}
	_, err := h.DB.ExecContext(ctx, `INSERT INTO client_report_outbox (
  ingest_id, source_event_id, client_event_id, employee_id, received_at, payload_json
) VALUES (?, ?, ?, ?, ?, ?)`,
		event.IngestID,
		toNullString(event.SourceEventID),
		toNullString(event.ClientEventID),
		event.EmployeeID,
		parseIngestEventTime(event.ReceivedAt),
		payload,
	)
	return err
}

func (h *Handler) insertStatDelta(ctx context.Context, eventKey string, ingestID string, sourceEventID string, employeeID int64, statDate time.Time, increments dailyIncrement) (bool, error) {
	if h.DB == nil {
		return true, nil
	}
	deltaKey := fmt.Sprintf("%s:%s", eventKey, statDate.Format("2006-01-02"))
	result, err := h.DB.ExecContext(ctx, `INSERT IGNORE INTO stat_deltas (
  event_key,
  ingest_id,
  source_event_id,
  stat_date,
  employee_id,
  work_seconds,
  normal_seconds,
  fish_seconds,
  idle_seconds,
  offline_seconds,
  attendance_seconds,
  effective_seconds
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		deltaKey,
		ingestID,
		toNullString(sourceEventID),
		statDate,
		employeeID,
		increments.Work,
		increments.Normal,
		increments.Fish,
		increments.Idle,
		increments.Offline,
		increments.Attendance,
		increments.Effective,
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (h *Handler) publishClientReportEvent(ctx context.Context, ingestID string, payload []byte) error {
	if h.MQ == nil {
		return mq.ErrDisabled
	}
	return h.MQ.Publish(ctx, mq.Message{
		Key:   ingestID,
		Value: payload,
	})
}

func (h *Handler) markClientReportOutboxPublished(ctx context.Context, ingestID string, publishedAt time.Time) error {
	if h.DB == nil {
		return nil
	}
	_, err := h.DB.ExecContext(ctx, `UPDATE client_report_outbox
SET mq_status = 'published', mq_attempts = mq_attempts + 1, last_error = NULL, published_at = ?
WHERE ingest_id = ?`, publishedAt, ingestID)
	return err
}

func (h *Handler) markClientReportOutboxFailed(ctx context.Context, ingestID string, publishErr error) error {
	if h.DB == nil {
		return nil
	}
	message := ""
	if publishErr != nil {
		message = publishErr.Error()
	}
	status := "failed"
	if errors.Is(publishErr, mq.ErrDisabled) {
		status = "pending"
	}
	_, err := h.DB.ExecContext(ctx, `UPDATE client_report_outbox
SET mq_status = ?, mq_attempts = mq_attempts + 1, last_error = ?
WHERE ingest_id = ?`, status, toNullString(message), ingestID)
	return err
}

func (h *Handler) relayClientReportOutbox(ctx context.Context, limit int) {
	if h.DB == nil || h.MQ == nil || h.Config == nil || !h.Config.MQ.Enabled {
		return
	}
	rows, err := h.DB.QueryContext(ctx, `SELECT ingest_id, payload_json
FROM client_report_outbox
WHERE mq_status IN ('pending', 'failed')
ORDER BY created_at ASC
LIMIT ?`, limit)
	if err != nil {
		log.Printf("outbox relay 查询失败: %v", err)
		return
	}
	defer rows.Close()

	type item struct {
		ingestID string
		payload  []byte
	}
	items := make([]item, 0, limit)
	for rows.Next() {
		var item item
		if err := rows.Scan(&item.ingestID, &item.payload); err != nil {
			log.Printf("outbox relay 读取失败: %v", err)
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		log.Printf("outbox relay 遍历失败: %v", err)
		return
	}

	for _, item := range items {
		if err := h.publishClientReportEvent(ctx, item.ingestID, item.payload); err != nil {
			_ = h.markClientReportOutboxFailed(ctx, item.ingestID, err)
			continue
		}
		_ = h.markClientReportOutboxPublished(ctx, item.ingestID, time.Now())
	}
}

func parseIngestEventTime(value string) time.Time {
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", value, time.Local)
	if err != nil {
		return time.Now()
	}
	return parsed
}
