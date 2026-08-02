package sqlc

import (
	"strings"
	"testing"
)

func TestDailyStatsQueryUsesEmployeeScopedHistoryLookups(t *testing.T) {
	for _, required := range []string{
		"ts.employee_id = ds.employee_id",
		"ws.employee_id = ds.employee_id",
		"time_segments ts FORCE INDEX (idx_time_segments_employee_end)",
		"work_sessions ws FORCE INDEX (idx_work_sessions_employee_end)",
		"ws.end_at IS NULL OR ws.end_at > ?",
	} {
		if !strings.Contains(listDailyStatsByDate, required) {
			t.Fatalf("日报查询缺少按员工缩小历史数据范围的条件 %q", required)
		}
	}
	for _, forbidden := range []string{
		"GROUP BY ts.employee_id",
		"GROUP BY ws.employee_id",
	} {
		if strings.Contains(listDailyStatsByDate, forbidden) {
			t.Fatalf("日报查询不应再对整张历史表先分组扫描: %q", forbidden)
		}
	}
	if count := strings.Count(listDailyStatsByDate, "?"); count != 12 {
		t.Fatalf("日报查询参数数量 = %d，期望 12", count)
	}
}

func TestRankQueryDoesNotReadTimeDetailTables(t *testing.T) {
	for _, forbidden := range []string{
		"time_segments",
		"work_sessions",
	} {
		if strings.Contains(listDailyStatsForRankByDate, forbidden) {
			t.Fatalf("排行查询不应读取明细表: %q", forbidden)
		}
	}
	if !strings.Contains(listDailyStatsForRankByDate, "WHERE ds.stat_date = ?") {
		t.Fatal("排行查询必须只读取指定日期的汇总数据")
	}
}

func TestDailyExportQueryUsesEmployeeScopedIndexLookups(t *testing.T) {
	for _, required := range []string{
		"time_segments ts FORCE INDEX (idx_time_segments_employee_end)",
		"ts.employee_id = ds.employee_id",
		"work_sessions ws FORCE INDEX (idx_work_sessions_employee_end)",
		"ws.employee_id = ds.employee_id",
		"work_sessions ws_start FORCE INDEX (idx_work_sessions_employee_start)",
		"ws_start.employee_id = ds.employee_id",
		"work_sessions ws_end FORCE INDEX (idx_work_sessions_employee_end)",
		"ws_end.employee_id = ds.employee_id",
	} {
		if !strings.Contains(listDailyStatsForExportByDate, required) {
			t.Fatalf("日报导出查询缺少按员工索引查找的条件 %q", required)
		}
	}
	for _, forbidden := range []string{
		"GROUP BY ts.employee_id",
		"GROUP BY ws.employee_id",
		"COALESCE(ws.end_at, ?) > ?",
	} {
		if strings.Contains(listDailyStatsForExportByDate, forbidden) {
			t.Fatalf("日报导出查询不应再扫描全部历史明细: %q", forbidden)
		}
	}
	if count := strings.Count(listDailyStatsForExportByDate, "?"); count != 16 {
		t.Fatalf("日报导出查询参数数量 = %d，期望 16", count)
	}
}
