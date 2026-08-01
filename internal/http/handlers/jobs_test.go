package handlers

import (
	"strings"
	"testing"
	"time"
)

func TestHistoryCleanupIsDisabledByDefault(t *testing.T) {
	settings := defaultSettings()
	if settings.HistoryCleanupEnabled != 0 {
		t.Fatal("历史数据定时清理必须默认关闭")
	}
	if settings.HistoryRetentionDays != 40 || settings.HistoryCleanupHour != 3 {
		t.Fatalf("默认清理配置错误: 保留天数=%d 执行小时=%d", settings.HistoryRetentionDays, settings.HistoryCleanupHour)
	}
}

func TestCleanupStatusRanOnDate(t *testing.T) {
	now := time.Date(2026, 8, 1, 8, 0, 0, 0, time.Local)
	if !cleanupStatusRanOnDate(HistoryCleanupStatus{LastRunAt: "2026-08-01 03:00:00"}, now) {
		t.Fatal("同一天已完成的清理不应重复运行")
	}
	if cleanupStatusRanOnDate(HistoryCleanupStatus{LastRunAt: "2026-07-31 23:59:59"}, now) {
		t.Fatal("前一天的清理不应阻止当天任务")
	}
	if cleanupStatusRanOnDate(HistoryCleanupStatus{LastRunAt: "无效时间"}, now) {
		t.Fatal("无效时间不应被误判为当天已运行")
	}
}

func TestHistoryCleanupPreservesReferencedSessions(t *testing.T) {
	var statement string
	for _, target := range historyCleanupTargets {
		if target.name == "无关联历史上班会话" {
			statement = target.statement
			break
		}
	}
	if statement == "" {
		t.Fatal("未找到上班会话清理规则")
	}
	for _, required := range []string{"work_session_checkouts", "work_session_reviews", "IS NULL"} {
		if !strings.Contains(statement, required) {
			t.Fatalf("上班会话清理规则缺少保护条件 %q", required)
		}
	}
}

func TestHistoryCleanupNeverDeletesAdministratorOrAuditData(t *testing.T) {
	statements := []string{
		`DELETE FROM time_segments WHERE employee_id = ? AND end_at < ?`,
	}
	for _, target := range historyCleanupTargets {
		statements = append(statements, target.statement)
	}
	joined := strings.ToLower(strings.Join(statements, "\n"))
	for _, protectedTable := range []string{"admin_users", "admin_sessions", "audit_logs", "departments", "employees"} {
		if strings.Contains(joined, "delete from "+protectedTable) {
			t.Fatalf("历史数据清理不得删除受保护表 %q", protectedTable)
		}
	}
}

func TestHistoryCleanupSkipsTablesWithoutSafeTimeIndex(t *testing.T) {
	joined := ""
	for _, target := range historyCleanupTargets {
		joined += "\n" + strings.ToLower(target.statement)
	}
	for _, skippedTable := range []string{"processed_source_events", "processed_ingests"} {
		if strings.Contains(joined, "delete from "+skippedTable) {
			t.Fatalf("缺少安全时间索引的表不得直接清理 %q", skippedTable)
		}
	}
}

func TestHistoryCleanupCanBeCanceled(t *testing.T) {
	canceled := false
	h := &Handler{
		historyCleanupState: HistoryCleanupStatus{Running: true},
		historyCleanupCancel: func() {
			canceled = true
		},
	}
	if !h.cancelHistoryCleanup() || !canceled {
		t.Fatal("正在运行的历史数据清理应可被停止")
	}
	if h.historyCleanupState.Message != "正在停止历史数据清理..." {
		t.Fatalf("停止状态不明确: %q", h.historyCleanupState.Message)
	}
}

func TestHistoryCleanupStatusReturnsDetailsCopy(t *testing.T) {
	h := &Handler{historyCleanupState: HistoryCleanupStatus{Details: map[string]int64{"每日统计": 3}}}
	status := h.getHistoryCleanupStatus()
	status.Details["每日统计"] = 99
	if h.historyCleanupState.Details["每日统计"] != 3 {
		t.Fatal("对外返回的清理状态不应修改内部状态")
	}
}

func TestHistoryCleanupProgressUpdatesLiveStatus(t *testing.T) {
	h := &Handler{historyCleanupState: HistoryCleanupStatus{
		Running:   true,
		StartedAt: formatTime(time.Now().Add(-3 * time.Second)),
	}}
	details := map[string]int64{"时间段明细": 750}
	h.updateHistoryCleanupProgress("时间段明细", 0, 5, 750, 3, 2, 10, 750, 1000, details)
	details["时间段明细"] = 999

	status := h.getHistoryCleanupStatus()
	if status.CurrentTarget != "时间段明细" || status.CompletedTargets != 0 || status.TotalTargets != 5 {
		t.Fatalf("清理项目进度错误: %+v", status)
	}
	if status.CurrentTargetDeleted != 750 || status.CurrentTargetBatches != 3 || status.TotalDeleted != 750 || status.TotalCandidates != 1000 {
		t.Fatalf("清理数量进度错误: %+v", status)
	}
	if status.ProcessedEmployees != 2 || status.TotalEmployees != 10 {
		t.Fatalf("员工处理进度错误: %+v", status)
	}
	if status.Details["时间段明细"] != 750 {
		t.Fatal("清理进度必须保存分项数据副本")
	}
	if status.ElapsedSeconds < 2 {
		t.Fatalf("清理耗时未实时计算: %d", status.ElapsedSeconds)
	}
}

func TestHistoryCleanupCountsEveryDeletedTarget(t *testing.T) {
	if len(historyCleanupCountTargets) != len(historyCleanupTargets) {
		t.Fatalf("通用待清理统计项与通用删除项数量不一致: 统计=%d 删除=%d", len(historyCleanupCountTargets), len(historyCleanupTargets))
	}
	want := []string{"每日统计", "统计增量去重记录", "客户端上报转发记录", "无关联历史上班会话"}
	for index, name := range want {
		if historyCleanupCountTargets[index].name != name {
			t.Fatalf("第 %d 个待清理统计项错误: %q", index+1, historyCleanupCountTargets[index].name)
		}
	}
}

func TestHistoryCleanupRequiresTimeSegmentCleanupIndex(t *testing.T) {
	found := false
	for _, index := range historyCleanupRequiredIndexes {
		if index.table == "time_segments" && index.name == "idx_time_segments_employee_end" {
			found = true
			if index.columns != "employee_id,end_at,id" {
				t.Fatalf("时间段清理索引顺序错误: %q", index.columns)
			}
		}
	}
	if !found {
		t.Fatal("清理前必须校验员工和结束时间复合索引")
	}
}

func TestHistoryCleanupTimeSegmentsUsesEmployeeEndTimeIndex(t *testing.T) {
	for _, required := range []string{"idx_time_segments_employee_end", "employee_id = ?", "end_at < ?"} {
		if !strings.Contains(historyCleanupTimeSegmentsCountSQL, required) {
			t.Fatalf("时间段统计语句缺少 %q", required)
		}
		if !strings.Contains(historyCleanupTimeSegmentsDeleteSQL, required) {
			t.Fatalf("时间段清理语句缺少 %q", required)
		}
	}
	for _, required := range []string{"ORDER BY end_at, id", "LIMIT ?"} {
		if !strings.Contains(historyCleanupTimeSegmentsDeleteSQL, required) {
			t.Fatalf("时间段分批清理语句缺少 %q", required)
		}
	}
}
