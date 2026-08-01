package sqlc

import (
	"strings"
	"testing"
)

func TestDailyStatsQueryUsesEmployeeScopedHistoryLookups(t *testing.T) {
	for _, required := range []string{
		"ts.employee_id = ds.employee_id",
		"ws.employee_id = ds.employee_id",
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
	if count := strings.Count(listDailyStatsByDate, "?"); count != 13 {
		t.Fatalf("日报查询参数数量 = %d，期望 13", count)
	}
}
