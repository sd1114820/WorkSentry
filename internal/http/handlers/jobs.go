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

type offlineRefreshCandidate struct {
	ID               int64
	LastSeenAt       time.Time
	LastSegmentEndAt sql.NullTime
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
		segmentStart := employee.LastSeenAt
		if employee.LastSegmentEndAt.Valid && employee.LastSegmentEndAt.Time.After(segmentStart) {
			segmentStart = employee.LastSegmentEndAt.Time
		}
		if now.After(segmentStart) {
			h.createSegmentAndStatsByContext(ctx, employee.ID, segmentStart, now, "offline", "", "offline")
			_ = h.Queries.UpdateEmployeeLastSegmentEnd(ctx, sqlc.UpdateEmployeeLastSegmentEndParams{
				LastSegmentEndAt: sql.NullTime{Time: now, Valid: true},
				ID:               employee.ID,
			})
		}
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
				ID:               employee.ID,
				LastSeenAt:       employee.LastSeenAt.Time,
				LastSegmentEndAt: employee.LastSegmentEndAt,
			})
		}
		return items, nil
	}

	dbRows, err := h.DB.QueryContext(ctx, `SELECT e.id, e.last_seen_at, e.last_segment_end_at
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
		if err := dbRows.Scan(&item.ID, &item.LastSeenAt, &item.LastSegmentEndAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := dbRows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (h *Handler) cleanupRawEvents(ctx context.Context) {
	cutoff := time.Now().AddDate(0, 0, -7)
	if err := h.Queries.DeleteRawEventsBefore(ctx, cutoff); err != nil {
		log.Printf("原始流水清理失败: %v", err)
	}
}
