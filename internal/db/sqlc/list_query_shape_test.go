package sqlc

import (
	"strings"
	"testing"
)

func TestOfflineSegmentsQueryUsesEmployeeRangeIndexAndStableGrouping(t *testing.T) {
	for _, fragment := range []string{
		"STRAIGHT_JOIN time_segments",
		"ts.start_at < ?",
		"ts.end_at > ?",
		"e.employee_code = ?",
		"ORDER BY e.employee_code, ts.start_at",
	} {
		if !strings.Contains(listOfflineSegmentsByDate, fragment) {
			t.Fatalf("离线段查询缺少 %q", fragment)
		}
	}
	if strings.Contains(listOfflineSegmentsByDate, "FORCE INDEX") {
		t.Fatal("离线段查询不应强制依赖旧库可能缺失的索引")
	}
}

func TestAuditDateQueryDoesNotWrapIndexedColumnInDateFunction(t *testing.T) {
	if strings.Contains(listAuditLogsByRange, "DATE(") {
		t.Fatal("审计日期查询不应对索引列使用 DATE 函数")
	}
	for _, fragment := range []string{"created_at >= ?", "created_at < ?"} {
		if !strings.Contains(listAuditLogsByRange, fragment) {
			t.Fatalf("审计日期查询缺少 %q", fragment)
		}
	}
	if !strings.Contains(listAuditLogsByRange, "COALESCE(detail, JSON_OBJECT())") {
		t.Fatal("审计查询应兼容旧数据中为空的详情字段")
	}
}
