package handlers

import (
	"database/sql"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

func TestBuildOfflineSegmentsByDateParamsUsesOverlapOrder(t *testing.T) {
	start := time.Date(2026, 8, 2, 0, 0, 0, 0, time.Local)
	end := start.Add(24 * time.Hour)
	params := buildOfflineSegmentsByDateParams(start, end, "E1001")

	if !params.RangeStart.Equal(start) {
		t.Fatalf("范围开始 = %s，期望 %s", params.RangeStart, start)
	}
	if !params.RangeEnd.Equal(end) {
		t.Fatalf("范围结束 = %s，期望 %s", params.RangeEnd, end)
	}
	if params.EmployeeCodeFilter != "E1001" || params.EmployeeCode != "E1001" {
		t.Fatalf("员工筛选参数错误: %#v", params)
	}
}

func TestOfflineSegmentMergerCombinesAdjacentRowsBeforeBuildingResponse(t *testing.T) {
	start := time.Date(2026, 8, 2, 8, 0, 0, 0, time.Local)
	merger := offlineSegmentMerger{}
	if !merger.add("E1001", "测试员工", sql.NullString{}, start, start.Add(time.Minute)) {
		t.Fatal("第一条离线段不应超出上限")
	}
	if !merger.add("E1001", "测试员工", sql.NullString{}, start.Add(time.Minute), start.Add(2*time.Minute)) {
		t.Fatal("相邻离线段不应超出上限")
	}
	if len(merger.views) != 1 {
		t.Fatalf("合并后离线段数 = %d，期望 1", len(merger.views))
	}
	if merger.views[0].Duration != "00:02" {
		t.Fatalf("合并后时长 = %q，期望 00:02", merger.views[0].Duration)
	}
}

func TestMissingForcedIndexErrorOnlyMatchesMySQL1176(t *testing.T) {
	if !isMissingForcedIndexError(&mysql.MySQLError{Number: 1176, Message: "key does not exist"}) {
		t.Fatal("应识别强制索引缺失错误")
	}
	if isMissingForcedIndexError(&mysql.MySQLError{Number: 1040, Message: "too many connections"}) {
		t.Fatal("连接数过多不应被当成索引缺失")
	}
}
