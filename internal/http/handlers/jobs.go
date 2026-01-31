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

func (h *Handler) refreshOfflineSegments(ctx context.Context) {
	settings := h.getSettingsOrDefaultByContext(ctx)
	threshold := time.Duration(settings.OfflineThresholdSeconds) * time.Second
	if threshold <= 0 {
		return
	}

	now := time.Now()
	employees, err := h.Queries.ListEmployeesForOfflineRefresh(ctx)
	if err != nil {
		log.Printf("离线刷新失败: %v", err)
		return
	}

	for _, employee := range employees {
		if !employee.LastSeenAt.Valid {
			continue
		}
		gap := now.Sub(employee.LastSeenAt.Time)
		if gap <= threshold {
			continue
		}

		segmentStart := employee.LastSeenAt.Time
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

func (h *Handler) cleanupRawEvents(ctx context.Context) {
	cutoff := time.Now().AddDate(0, 0, -7)
	if err := h.Queries.DeleteRawEventsBefore(ctx, cutoff); err != nil {
		log.Printf("原始流水清理失败: %v", err)
	}
}
