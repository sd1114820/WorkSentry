package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"worksentry/internal/db/sqlc"
)

func (h *Handler) StartBackgroundJobs(ctx context.Context) {
	go h.offlineRefreshLoop(ctx)
	go h.rawCleanupLoop(ctx)
	go h.outboxRelayLoop(ctx)
	go h.historyCleanupSchedulerLoop(ctx)
	log.Printf("历史数据清理由管理页面控制，服务启动时不执行清理")
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
	deleted, err := h.cleanupHistoryTarget(ctx, historyCleanupTarget{
		name: "原始上报流水",
		statement: `DELETE FROM raw_events
WHERE received_at < ?
ORDER BY received_at, id
LIMIT ?`,
	}, cutoff, historyCleanupBatchSize, nil)
	if err != nil {
		log.Printf("原始流水清理失败: %v", err)
		return
	}
	log.Printf("原始流水清理完成: 删除=%d", deleted)
}

type historyCleanupTarget struct {
	name      string
	statement string
}

const historyCleanupBatchSize = 250

const historyCleanupTimeSegmentsCountSQL = `SELECT COUNT(*)
FROM time_segments FORCE INDEX (idx_time_segments_employee_end)
WHERE employee_id = ?
  AND end_at < ?`

const historyCleanupTimeSegmentsDeleteSQL = `DELETE FROM time_segments
WHERE id IN (
  SELECT id FROM (
    SELECT id
    FROM time_segments FORCE INDEX (idx_time_segments_employee_end)
    WHERE employee_id = ?
      AND end_at < ?
    ORDER BY end_at, id
    LIMIT ?
  ) cleanup_candidates
)`

type HistoryCleanupStatus struct {
	Running              bool             `json:"running"`
	Trigger              string           `json:"trigger,omitempty"`
	Message              string           `json:"message"`
	StartedAt            string           `json:"startedAt,omitempty"`
	CompletedAt          string           `json:"completedAt,omitempty"`
	LastRunAt            string           `json:"lastRunAt,omitempty"`
	CutoffDate           string           `json:"cutoffDate,omitempty"`
	RetentionDays        int              `json:"retentionDays"`
	TotalDeleted         int64            `json:"totalDeleted"`
	TotalCandidates      int64            `json:"totalCandidates"`
	CurrentTarget        string           `json:"currentTarget,omitempty"`
	CurrentTargetDeleted int64            `json:"currentTargetDeleted"`
	CurrentTargetBatches int              `json:"currentTargetBatches"`
	CompletedTargets     int              `json:"completedTargets"`
	TotalTargets         int              `json:"totalTargets"`
	ProcessedEmployees   int              `json:"processedEmployees"`
	TotalEmployees       int              `json:"totalEmployees"`
	ElapsedSeconds       int64            `json:"elapsedSeconds"`
	Details              map[string]int64 `json:"details,omitempty"`
	Warnings             []string         `json:"warnings,omitempty"`
}

type historyCleanupResult struct {
	cutoff          time.Time
	totalDeleted    int64
	totalCandidates int64
	details         map[string]int64
	warnings        []string
}

var historyCleanupWarnings = []string{
	"已跳过已处理上报记录和已处理批次记录：现有表缺少按时间清理的索引，直接删除可能再次阻塞服务",
}

var historyCleanupTargets = []historyCleanupTarget{
	{
		name: "每日统计",
		statement: `DELETE FROM daily_stats
WHERE stat_date < DATE(?)
ORDER BY stat_date, employee_id
LIMIT ?`,
	},
	{
		name: "统计增量去重记录",
		statement: `DELETE FROM stat_deltas
WHERE stat_date < DATE(?)
ORDER BY stat_date, employee_id
LIMIT ?`,
	},
	{
		name: "客户端上报转发记录",
		statement: `DELETE FROM client_report_outbox
WHERE mq_status = 'published'
  AND created_at < ?
ORDER BY created_at
LIMIT ?`,
	},
	{
		name: "无关联历史上班会话",
		statement: `DELETE FROM work_sessions
WHERE id IN (
  SELECT id FROM (
    SELECT ws.id
    FROM work_sessions ws
    LEFT JOIN work_session_checkouts checkout_record ON checkout_record.work_session_id = ws.id
    LEFT JOIN work_session_reviews review_record ON review_record.work_session_id = ws.id
    WHERE ws.end_at IS NOT NULL
      AND ws.end_at < ?
      AND checkout_record.id IS NULL
      AND review_record.id IS NULL
    ORDER BY ws.end_at, ws.id
    LIMIT ?
  ) cleanup_candidates
)`,
	},
}

var historyCleanupCountTargets = []historyCleanupTarget{
	{
		name: "每日统计",
		statement: `SELECT COUNT(*)
FROM daily_stats FORCE INDEX (uk_daily_stats_date_employee)
WHERE stat_date < DATE(?)`,
	},
	{
		name: "统计增量去重记录",
		statement: `SELECT COUNT(*)
FROM stat_deltas FORCE INDEX (idx_stat_deltas_date_employee)
WHERE stat_date < DATE(?)`,
	},
	{
		name: "客户端上报转发记录",
		statement: `SELECT COUNT(*)
FROM client_report_outbox FORCE INDEX (idx_client_report_outbox_status_created)
WHERE mq_status = 'published'
  AND created_at < ?`,
	},
	{
		name: "无关联历史上班会话",
		statement: `SELECT COUNT(*)
FROM work_sessions ws FORCE INDEX (idx_work_sessions_end_employee_start)
LEFT JOIN work_session_checkouts checkout_record ON checkout_record.work_session_id = ws.id
LEFT JOIN work_session_reviews review_record ON review_record.work_session_id = ws.id
WHERE ws.end_at IS NOT NULL
  AND ws.end_at < ?
  AND checkout_record.id IS NULL
  AND review_record.id IS NULL`,
	},
}

type historyCleanupIndex struct {
	table   string
	name    string
	columns string
}

var historyCleanupRequiredIndexes = []historyCleanupIndex{
	{table: "time_segments", name: "idx_time_segments_employee_end", columns: "employee_id,end_at,id"},
	{table: "daily_stats", name: "uk_daily_stats_date_employee", columns: "stat_date,employee_id"},
	{table: "stat_deltas", name: "idx_stat_deltas_date_employee", columns: "stat_date,employee_id"},
	{table: "client_report_outbox", name: "idx_client_report_outbox_status_created", columns: "mq_status,created_at"},
	{table: "work_sessions", name: "idx_work_sessions_end_employee_start", columns: "end_at,employee_id,start_at"},
	{table: "work_session_checkouts", name: "uk_work_session_checkouts_session", columns: "work_session_id"},
	{table: "work_session_reviews", name: "uk_work_session_reviews_session", columns: "work_session_id"},
}

func (h *Handler) historyCleanupSchedulerLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			h.maybeStartScheduledHistoryCleanup(ctx, now)
		}
	}
}

func (h *Handler) maybeStartScheduledHistoryCleanup(ctx context.Context, now time.Time) {
	settings := h.getSettingsOrDefaultByContext(ctx)
	if settings.HistoryCleanupEnabled <= 0 || int(settings.HistoryCleanupHour) != now.Hour() {
		return
	}
	if settings.HistoryCleanupLastRunAt.Valid && isSameLocalDate(settings.HistoryCleanupLastRunAt.Time, now) {
		return
	}
	status := h.getHistoryCleanupStatus()
	if status.Running || cleanupStatusRanOnDate(status, now) {
		return
	}
	h.startHistoryCleanup(ctx, "定时", int(settings.HistoryRetentionDays))
}

func cleanupStatusRanOnDate(status HistoryCleanupStatus, now time.Time) bool {
	if status.LastRunAt == "" {
		return false
	}
	lastRunAt, err := parseDateTime(status.LastRunAt)
	return err == nil && isSameLocalDate(lastRunAt, now)
}

func (h *Handler) startHistoryCleanup(parent context.Context, trigger string, retentionDays int) bool {
	if h.DB == nil {
		return false
	}
	if retentionDays < 1 || retentionDays > 3650 {
		return false
	}

	now := time.Now()
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -retentionDays)
	runContext, cancel := context.WithCancel(context.WithoutCancel(parent))
	h.historyCleanupMu.Lock()
	if h.historyCleanupState.Running {
		h.historyCleanupMu.Unlock()
		cancel()
		return false
	}
	h.historyCleanupState = HistoryCleanupStatus{
		Running:       true,
		Trigger:       trigger,
		Message:       fmt.Sprintf("正在准备清理 %d 天以前的历史数据", retentionDays),
		StartedAt:     formatTime(now),
		CutoffDate:    cutoff.Format("2006-01-02"),
		RetentionDays: retentionDays,
		TotalTargets:  len(historyCleanupTargets) + 1,
	}
	h.historyCleanupCancel = cancel
	h.historyCleanupMu.Unlock()

	go func() {
		defer cancel()

		result, err := h.executeHistoryCleanup(runContext, retentionDays, historyCleanupBatchSize)
		completedAt := time.Now()
		progress := h.getHistoryCleanupStatus()
		status := HistoryCleanupStatus{
			Running:              false,
			Trigger:              trigger,
			StartedAt:            formatTime(now),
			CompletedAt:          formatTime(completedAt),
			CutoffDate:           cutoff.Format("2006-01-02"),
			RetentionDays:        retentionDays,
			TotalDeleted:         result.totalDeleted,
			TotalCandidates:      result.totalCandidates,
			Details:              result.details,
			Warnings:             append([]string(nil), result.warnings...),
			CurrentTarget:        progress.CurrentTarget,
			CurrentTargetDeleted: progress.CurrentTargetDeleted,
			CurrentTargetBatches: progress.CurrentTargetBatches,
			CompletedTargets:     progress.CompletedTargets,
			TotalTargets:         progress.TotalTargets,
			ProcessedEmployees:   progress.ProcessedEmployees,
			TotalEmployees:       progress.TotalEmployees,
			ElapsedSeconds:       int64(time.Since(now).Seconds()),
		}
		if errors.Is(err, context.Canceled) {
			status.Message = fmt.Sprintf("历史数据清理已停止，停止前删除 %d 条", result.totalDeleted)
			log.Printf("历史数据清理已停止: 触发方式=%s 已删除=%d", trigger, result.totalDeleted)
		} else if err != nil {
			status.Message = fmt.Sprintf("历史数据清理失败，失败前删除 %d 条：%s", result.totalDeleted, err.Error())
			log.Printf("历史数据清理失败: 触发方式=%s 错误=%v", trigger, err)
		} else {
			status.Message = fmt.Sprintf("历史数据清理完成，共删除 %d 条", result.totalDeleted)
			status.CurrentTarget = ""
			status.CurrentTargetDeleted = 0
			status.CurrentTargetBatches = 0
			status.ProcessedEmployees = 0
			status.TotalEmployees = 0
			status.CompletedTargets = status.TotalTargets
			if len(result.warnings) > 0 {
				status.Message += "；有数据因缺少安全索引已跳过"
			}
			status.LastRunAt = formatTime(completedAt)
			status.CutoffDate = result.cutoff.Format("2006-01-02")
			h.recordHistoryCleanupSuccess(completedAt)
			log.Printf("历史数据清理完成: 触发方式=%s 共删除=%d", trigger, result.totalDeleted)
		}
		h.historyCleanupMu.Lock()
		h.historyCleanupState = status
		h.historyCleanupCancel = nil
		h.historyCleanupMu.Unlock()
	}()

	return true
}

func (h *Handler) cancelHistoryCleanup() bool {
	h.historyCleanupMu.Lock()
	defer h.historyCleanupMu.Unlock()
	if !h.historyCleanupState.Running || h.historyCleanupCancel == nil {
		return false
	}
	h.historyCleanupState.Message = "正在停止历史数据清理..."
	h.historyCleanupCancel()
	return true
}

func (h *Handler) executeHistoryCleanup(ctx context.Context, retentionDays int, batchSize int) (historyCleanupResult, error) {
	result := historyCleanupResult{
		details:  make(map[string]int64, len(historyCleanupTargets)+1),
		warnings: append([]string(nil), historyCleanupWarnings...),
	}
	if h.DB == nil {
		return result, fmt.Errorf("数据库未初始化")
	}
	if retentionDays < 1 || retentionDays > 3650 || batchSize <= 0 {
		return result, fmt.Errorf("清理配置无效")
	}

	startedAt := time.Now()
	todayStart := time.Date(startedAt.Year(), startedAt.Month(), startedAt.Day(), 0, 0, 0, 0, startedAt.Location())
	cutoff := todayStart.AddDate(0, 0, -retentionDays)
	result.cutoff = cutoff
	log.Printf("历史数据清理开始: 截止时间=%s", cutoff.Format("2006-01-02 15:04:05"))

	if err := h.verifyHistoryCleanupIndexes(ctx); err != nil {
		return result, err
	}
	totalCandidates, timeSegmentEmployeeIDs, err := h.countHistoryCleanupCandidates(ctx, cutoff)
	result.totalCandidates = totalCandidates
	if err != nil {
		return result, err
	}

	totalTargets := len(historyCleanupTargets) + 1
	h.updateHistoryCleanupProgress("时间段明细", 0, totalTargets, 0, 0, 0, len(timeSegmentEmployeeIDs), result.totalDeleted, result.totalCandidates, result.details)
	timeSegmentsDeleted, err := h.cleanupTimeSegmentsByEmployee(ctx, cutoff, batchSize, timeSegmentEmployeeIDs, func(deleted int64, batches int, processedEmployees int, totalEmployees int) {
		result.details["时间段明细"] = deleted
		h.updateHistoryCleanupProgress("时间段明细", 0, totalTargets, deleted, batches, processedEmployees, totalEmployees, result.totalDeleted+deleted, result.totalCandidates, result.details)
	})
	result.totalDeleted += timeSegmentsDeleted
	result.details["时间段明细"] = timeSegmentsDeleted
	if err != nil {
		return result, fmt.Errorf("清理时间段明细失败: %w", err)
	}
	log.Printf("历史数据清理分项完成: 数据=时间段明细 删除=%d", timeSegmentsDeleted)

	for index, target := range historyCleanupTargets {
		completedTargets := index + 1
		h.updateHistoryCleanupProgress(target.name, completedTargets, totalTargets, 0, 0, 0, 0, result.totalDeleted, result.totalCandidates, result.details)
		deleted, err := h.cleanupHistoryTarget(ctx, target, cutoff, batchSize, func(deleted int64, batches int) {
			result.details[target.name] = deleted
			h.updateHistoryCleanupProgress(target.name, completedTargets, totalTargets, deleted, batches, 0, 0, result.totalDeleted+deleted, result.totalCandidates, result.details)
		})
		result.totalDeleted += deleted
		result.details[target.name] = deleted
		if err != nil {
			return result, fmt.Errorf("清理%s失败: %w", target.name, err)
		}
		h.updateHistoryCleanupProgress(target.name, completedTargets+1, totalTargets, deleted, 0, 0, 0, result.totalDeleted, result.totalCandidates, result.details)
		log.Printf("历史数据清理分项完成: 数据=%s 删除=%d", target.name, deleted)
	}

	log.Printf("历史数据清理执行完成: 共删除=%d 耗时=%s", result.totalDeleted, time.Since(startedAt).Round(time.Millisecond))
	return result, nil
}

func (h *Handler) verifyHistoryCleanupIndexes(ctx context.Context) error {
	for _, index := range historyCleanupRequiredIndexes {
		columns, err := h.historyCleanupIndexColumns(ctx, index)
		if err != nil {
			return err
		}
		if columns != index.columns {
			return fmt.Errorf("清理索引 %s.%s 结构不正确，期望=%s 实际=%s", index.table, index.name, index.columns, columns)
		}
	}
	return nil
}

func (h *Handler) historyCleanupIndexColumns(ctx context.Context, index historyCleanupIndex) (string, error) {
	var columns sql.NullString
	err := h.DB.QueryRowContext(ctx, `SELECT GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX SEPARATOR ',')
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = ?
  AND INDEX_NAME = ?`, index.table, index.name).Scan(&columns)
	if err != nil {
		return "", fmt.Errorf("检查清理索引 %s.%s 失败: %w", index.table, index.name, err)
	}
	if !columns.Valid {
		return "", nil
	}
	return columns.String, nil
}

func (h *Handler) countHistoryCleanupCandidates(ctx context.Context, cutoff time.Time) (int64, []int64, error) {
	timeSegmentTotal, employeeIDs, err := h.countTimeSegmentCleanupCandidates(ctx, cutoff)
	if err != nil {
		return 0, nil, err
	}
	total := timeSegmentTotal
	totalTargets := len(historyCleanupCountTargets) + 1
	h.updateHistoryCleanupCountProgress("时间段明细", 1, totalTargets, total)
	for index, target := range historyCleanupCountTargets {
		h.updateHistoryCleanupCountProgress(target.name, index+1, totalTargets, total)
		var count int64
		if err := h.DB.QueryRowContext(ctx, target.statement, cutoff).Scan(&count); err != nil {
			return total, employeeIDs, fmt.Errorf("统计%s待清理数量失败: %w", target.name, err)
		}
		total += count
		h.updateHistoryCleanupCountProgress(target.name, index+2, totalTargets, total)
	}
	return total, employeeIDs, nil
}

func (h *Handler) countTimeSegmentCleanupCandidates(ctx context.Context, cutoff time.Time) (int64, []int64, error) {
	rows, err := h.DB.QueryContext(ctx, `SELECT id FROM employees ORDER BY id`)
	if err != nil {
		return 0, nil, fmt.Errorf("读取员工列表失败: %w", err)
	}
	allEmployeeIDs := make([]int64, 0, 128)
	for rows.Next() {
		var employeeID int64
		if err := rows.Scan(&employeeID); err != nil {
			_ = rows.Close()
			return 0, nil, fmt.Errorf("读取员工编号失败: %w", err)
		}
		allEmployeeIDs = append(allEmployeeIDs, employeeID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, nil, fmt.Errorf("读取员工列表失败: %w", err)
	}
	if err := rows.Close(); err != nil {
		return 0, nil, fmt.Errorf("关闭员工列表失败: %w", err)
	}

	var total int64
	candidateEmployeeIDs := make([]int64, 0, len(allEmployeeIDs))
	for employeeIndex, employeeID := range allEmployeeIDs {
		h.updateHistoryCleanupEmployeeCountProgress(employeeIndex, len(allEmployeeIDs), total)
		var count int64
		if err := h.DB.QueryRowContext(ctx, historyCleanupTimeSegmentsCountSQL, employeeID, cutoff).Scan(&count); err != nil {
			return total, candidateEmployeeIDs, fmt.Errorf("统计员工编号=%d 的时间段明细失败: %w", employeeID, err)
		}
		if count > 0 {
			candidateEmployeeIDs = append(candidateEmployeeIDs, employeeID)
			total += count
		}
		h.updateHistoryCleanupEmployeeCountProgress(employeeIndex+1, len(allEmployeeIDs), total)
	}
	return total, candidateEmployeeIDs, nil
}

func (h *Handler) updateHistoryCleanupEmployeeCountProgress(processedEmployees int, totalEmployees int, totalCandidates int64) {
	h.historyCleanupMu.Lock()
	defer h.historyCleanupMu.Unlock()
	if !h.historyCleanupState.Running {
		return
	}
	h.historyCleanupState.CurrentTarget = "统计待清理数据：时间段明细"
	h.historyCleanupState.ProcessedEmployees = processedEmployees
	h.historyCleanupState.TotalEmployees = totalEmployees
	h.historyCleanupState.TotalCandidates = totalCandidates
	h.historyCleanupState.Message = fmt.Sprintf("正在按员工索引统计时间段明细：已统计 %d/%d 位，已发现 %d 条", processedEmployees, totalEmployees, totalCandidates)
}

func (h *Handler) updateHistoryCleanupCountProgress(target string, countedTargets int, totalTargets int, totalCandidates int64) {
	h.historyCleanupMu.Lock()
	defer h.historyCleanupMu.Unlock()
	if !h.historyCleanupState.Running {
		return
	}
	h.historyCleanupState.CurrentTarget = "统计待清理数据：" + target
	h.historyCleanupState.TotalCandidates = totalCandidates
	h.historyCleanupState.ProcessedEmployees = 0
	h.historyCleanupState.TotalEmployees = 0
	h.historyCleanupState.Message = fmt.Sprintf("正在统计待清理数据：已统计 %d/%d 项，已发现 %d 条", countedTargets, totalTargets, totalCandidates)
}

func (h *Handler) cleanupTimeSegmentsByEmployee(ctx context.Context, cutoff time.Time, batchSize int, employeeIDs []int64, reportProgress func(deleted int64, batches int, processedEmployees int, totalEmployees int)) (int64, error) {
	var totalDeleted int64
	var batches int
	if reportProgress != nil {
		reportProgress(0, 0, 0, len(employeeIDs))
	}
	for employeeIndex, employeeID := range employeeIDs {
		for {
			result, err := h.DB.ExecContext(ctx, historyCleanupTimeSegmentsDeleteSQL, employeeID, cutoff, batchSize)
			if err != nil {
				return totalDeleted, fmt.Errorf("员工编号=%d 执行分批删除失败: %w", employeeID, err)
			}
			deleted, err := result.RowsAffected()
			if err != nil {
				return totalDeleted, fmt.Errorf("员工编号=%d 读取删除数量失败: %w", employeeID, err)
			}
			totalDeleted += deleted
			if deleted > 0 {
				batches++
			}
			processedEmployees := employeeIndex
			if deleted < int64(batchSize) {
				processedEmployees++
			}
			if reportProgress != nil {
				reportProgress(totalDeleted, batches, processedEmployees, len(employeeIDs))
			}
			if deleted < int64(batchSize) {
				break
			}
		}
	}
	return totalDeleted, nil
}

func (h *Handler) cleanupHistoryTarget(ctx context.Context, target historyCleanupTarget, cutoff time.Time, batchSize int, reportProgress func(deleted int64, batches int)) (int64, error) {
	var totalDeleted int64
	var batches int
	for {
		result, err := h.DB.ExecContext(ctx, target.statement, cutoff, batchSize)
		if err != nil {
			return totalDeleted, fmt.Errorf("执行分批删除失败: %w", err)
		}
		deleted, err := result.RowsAffected()
		if err != nil {
			return totalDeleted, fmt.Errorf("读取删除数量失败: %w", err)
		}
		totalDeleted += deleted
		if deleted > 0 {
			batches++
		}
		if reportProgress != nil {
			reportProgress(totalDeleted, batches)
		}
		if deleted < int64(batchSize) {
			return totalDeleted, nil
		}
	}
}

func (h *Handler) updateHistoryCleanupProgress(target string, completedTargets int, totalTargets int, targetDeleted int64, targetBatches int, processedEmployees int, totalEmployees int, totalDeleted int64, totalCandidates int64, details map[string]int64) {
	h.historyCleanupMu.Lock()
	defer h.historyCleanupMu.Unlock()
	if !h.historyCleanupState.Running {
		return
	}
	h.historyCleanupState.CurrentTarget = target
	h.historyCleanupState.CurrentTargetDeleted = targetDeleted
	h.historyCleanupState.CurrentTargetBatches = targetBatches
	h.historyCleanupState.CompletedTargets = completedTargets
	h.historyCleanupState.TotalTargets = totalTargets
	h.historyCleanupState.ProcessedEmployees = processedEmployees
	h.historyCleanupState.TotalEmployees = totalEmployees
	h.historyCleanupState.TotalDeleted = totalDeleted
	h.historyCleanupState.TotalCandidates = totalCandidates
	h.historyCleanupState.Details = cloneCleanupDetails(details)
	h.historyCleanupState.Message = fmt.Sprintf("正在清理%s：已完成 %d/%d 项，已删除 %d/%d 条", target, completedTargets, totalTargets, totalDeleted, totalCandidates)
}

func (h *Handler) getHistoryCleanupStatus() HistoryCleanupStatus {
	h.historyCleanupMu.RLock()
	defer h.historyCleanupMu.RUnlock()
	status := h.historyCleanupState
	if status.Details != nil {
		status.Details = cloneCleanupDetails(status.Details)
	}
	status.Warnings = append([]string(nil), status.Warnings...)
	if status.Running && status.StartedAt != "" {
		if startedAt, err := parseDateTime(status.StartedAt); err == nil {
			status.ElapsedSeconds = int64(time.Since(startedAt).Seconds())
		}
	}
	return status
}

func cloneCleanupDetails(source map[string]int64) map[string]int64 {
	result := make(map[string]int64, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func (h *Handler) recordHistoryCleanupSuccess(completedAt time.Time) {
	if h.DB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := h.DB.ExecContext(ctx, `UPDATE settings SET history_cleanup_last_run_at = ? WHERE id = 1`, completedAt); err != nil {
		log.Printf("记录历史数据清理时间失败: %v", err)
		return
	}
	h.invalidateSettingsCache()
}
