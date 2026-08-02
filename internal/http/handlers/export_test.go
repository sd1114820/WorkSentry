package handlers

import (
	"bytes"
	"database/sql"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"worksentry/internal/db/sqlc"
)

func TestBuildDailyExportWorkbook(t *testing.T) {
	firstStart := time.Date(2026, 8, 2, 9, 5, 0, 0, time.Local)
	lastEnd := time.Date(2026, 8, 2, 18, 6, 0, 0, time.Local)
	content, err := buildDailyExportWorkbook("2026-08-02", []sqlc.ListDailyStatsForExportByDateRow{{
		EmployeeCode:      "A001",
		Name:              "测试员工",
		DepartmentName:    sql.NullString{String: "研发部", Valid: true},
		NormalSeconds:     3600,
		FishSeconds:       600,
		IdleSeconds:       300,
		OfflineSeconds:    120,
		AttendanceSeconds: 28800,
		EffectiveSeconds:  7200,
		BreakSeconds:      180,
		OnDutySeconds:     32400,
		FirstStartAt:      firstStart,
		LastEndAt:         lastEnd,
	}})
	if err != nil {
		t.Fatalf("生成日报导出文件失败: %v", err)
	}
	if len(content) == 0 {
		t.Fatal("日报导出文件不应为空")
	}

	file, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("打开日报导出文件失败: %v", err)
	}
	defer func() { _ = file.Close() }()
	for cell, expected := range map[string]string{
		"A1": "日期",
		"A2": "2026-08-02",
		"B2": "A001",
		"C2": "测试员工",
		"D2": "研发部",
		"E2": "09:05",
		"F2": "18:06",
	} {
		actual, getErr := file.GetCellValue("日报表", cell)
		if getErr != nil {
			t.Fatalf("读取单元格 %s 失败: %v", cell, getErr)
		}
		if actual != expected {
			t.Fatalf("单元格 %s = %q，期望 %q", cell, actual, expected)
		}
	}
}
