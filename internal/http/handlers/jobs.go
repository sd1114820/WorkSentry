package handlers

import (
	"context"
	"database/sql"
	"log"
	"time"

	"worksentry/internal/db/sqlc"
)

func (h *Handler) StartBackgroundJobs(ctx context.Context) {
	go h.offlineRefreshLoop(ctx)
	go h.rawCleanupLoop(ctx)
	go h.outboxRelayLoop(ctx)
}

func (h *Handler) offlineRefreshLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.refreshOfflineSegments(ctx)
		}
	}
}

func (h *Handler) rawCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.cleanupRawEvents(ctx)
		}
	}
}

func (h *Handler) outboxRelayLoop(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.relayClientReportOutbox(ctx, 100)
		}
	}
}

type offlineRefreshCandidate struct {
	ID                      int64
	LastSeenAt              time.Time
	LastStatus              sqlc.NullEmployeesLastStatus
	LastDescription         sql.NullString
	CurrentSegmentStartedAt sql.NullTime
	LastSegmentEndAt        sql.NullTime
}

func (h *Handler) refreshOfflineSegments(ctx context.Context) {
	settings := h.getSettingsOrDefaultByContext(ctx)
	threshold := time.Duration(settings.OfflineThresholdSeconds) * time.Second
	if threshold <= 0 {
		return
	}

	now := time.Now()
	cutoff := now.Add(-threshold)

	employees, err := h.listOfflineRefreshCandidates(ctx, cutoff)
	if err != nil {
		log.Printf("离线刷新失败: %v", err)
		return
	}

	for _, employee := range employees {
		h.advanceOfflineState(ctx, employee, now)
	}
}

func (h *Handler) listOfflineRefreshCandidates(ctx context.Context, cutoff time.Time) ([]offlineRefreshCandidate, error) {
	if h.DB == nil {
		rows, err := h.Queries.ListEmployeesForOfflineRefresh(ctx)
		if err != nil {
			return nil, err
		}
		items := make([]offlineRefreshCandidate, 0, len(rows))
		for _, employee := range rows {
			if !employee.LastSeenAt.Valid {
				continue
			}
			if !employee.LastSeenAt.Time.Before(cutoff) {
				continue
			}
			if _, err := h.Queries.GetOpenWorkSessionByEmployee(ctx, employee.ID); err != nil {
				continue
			}
			items = append(items, offlineRefreshCandidate{
				ID:                      employee.ID,
				LastSeenAt:              employee.LastSeenAt.Time,
				LastStatus:              employee.LastStatus,
				LastDescription:         employee.LastDescription,
				CurrentSegmentStartedAt: employee.CurrentSegmentStartedAt,
				LastSegmentEndAt:        employee.LastSegmentEndAt,
			})
		}
		return items, nil
	}

	dbRows, err := h.DB.QueryContext(ctx, `SELECT e.id, e.last_seen_at, e.last_status, e.last_description, e.current_segment_started_at, e.last_segment_end_at
FROM employees e
JOIN (
  SELECT employee_id
  FROM work_sessions
  WHERE end_at IS NULL
  GROUP BY employee_id
) ws ON ws.employee_id = e.id
WHERE e.enabled = 1
  AND e.last_seen_at IS NOT NULL
  AND e.last_seen_at < ?`, cutoff)
	if err != nil {
		return nil, err
	}
	defer dbRows.Close()

	items := make([]offlineRefreshCandidate, 0, 64)
	for dbRows.Next() {
		var item offlineRefreshCandidate
		if err := dbRows.Scan(&item.ID, &item.LastSeenAt, &item.LastStatus, &item.LastDescription, &item.CurrentSegmentStartedAt, &item.LastSegmentEndAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := dbRows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (h *Handler) advanceOfflineState(ctx context.Context, employee offlineRefreshCandidate, now time.Time) {
	lastSeen := employee.LastSeenAt
	lastDescription := nullString(employee.LastDescription)
	lastStatus := ""
	if employee.LastStatus.Valid {
		lastStatus = string(employee.LastStatus.EmployeesLastStatus)
	}

	lastEnd := sql.NullTime{Time: lastSeen, Valid: true}
	if employee.LastSegmentEndAt.Valid {
		lastEnd = employee.LastSegmentEndAt
		if lastEnd.Time.Before(lastSeen) {
			lastEnd = sql.NullTime{Time: lastSeen, Valid: true}
		}
	}

	currentStart := employee.CurrentSegmentStartedAt
	if !currentStart.Valid {
		currentStart = sql.NullTime{Time: lastSeen, Valid: true}
	}

	if lastStatus == "offline" {
		h.addDailyStatsByRange(ctx, employee.ID, "offline", lastEnd.Time, now)
		h.updateEmployeeTrackingStateByContext(ctx, employee.ID, lastSeen, "offline", "离线", currentStart, sql.NullTime{Time: now, Valid: true})
		return
	}

	if lastStatus != "" && lastEnd.Time.Before(lastSeen) {
		h.addDailyStatsByRange(ctx, employee.ID, lastStatus, lastEnd.Time, lastSeen)
	}

	if lastStatus != "" && currentStart.Valid && lastSeen.After(currentStart.Time) {
		h.createTrackedSegmentOnlyByContext(ctx, employee.ID, currentStart.Time, lastSeen, lastStatus, lastDescription, sourceForTrackedStatus(lastStatus))
	}

	h.addDailyStatsByRange(ctx, employee.ID, "offline", lastSeen, now)
	h.createSparseRawEvent(ctx, employee.ID, now, "", "", 0, "offline", "", "")
	h.updateEmployeeTrackingStateByContext(ctx, employee.ID, lastSeen, "offline", "离线", sql.NullTime{Time: lastSeen, Valid: true}, sql.NullTime{Time: now, Valid: true})
}

func (h *Handler) cleanupRawEvents(ctx context.Context) {
	retentionDays := 3
	if h.Config != nil && h.Config.Database.RawEventsRetentionDays > 0 {
		retentionDays = h.Config.Database.RawEventsRetentionDays
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	if err := h.Queries.DeleteRawEventsBefore(ctx, cutoff); err != nil {
		log.Printf("原始流水清理失败: %v", err)
	}
}
