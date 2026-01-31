package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/xuri/excelize/v2"

	"worksentry/internal/db/sqlc"
)

func (h *Handler) ExportDaily(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
		return
	}
	dateValue := r.URL.Query().Get("date")
	if dateValue == "" {
		dateValue = time.Now().Format("2006-01-02")
	}
	date, err := parseDate(dateValue)
	if err != nil {
		writeError(w, http.StatusBadRequest, "日期格式错误")
		return
	}
	departmentID, _ := strconv.ParseInt(r.URL.Query().Get("departmentId"), 10, 64)
	rows, err := h.Queries.ListDailyStatsByDate(r.Context(), sqlc.ListDailyStatsByDateParams{
		StatDate:     date,
		Column2:      departmentID,
		DepartmentID: toNullInt64(departmentID),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "导出失败")
		return
	}

	file := excelize.NewFile()
	sheet := "日报表"
	file.SetSheetName("Sheet1", sheet)

	headers := []string{"日期", "工号", "姓名", "部门", "工作时长", "常规时长", "摸鱼时长", "离开时长", "离线时长", "在岗时长", "有效工时"}
	for col, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		_ = file.SetCellValue(sheet, cell, header)
	}

	for i, row := range rows {
		idx := i + 2
		values := []any{
			dateValue,
			row.EmployeeCode,
			row.Name,
			nullString(row.DepartmentName),
			formatDuration(int64(row.WorkSeconds)),
			formatDuration(int64(row.NormalSeconds)),
			formatDuration(int64(row.FishSeconds)),
			formatDuration(int64(row.IdleSeconds)),
			formatDuration(int64(row.OfflineSeconds)),
			formatDuration(int64(row.AttendanceSeconds)),
			formatDuration(int64(row.EffectiveSeconds)),
		}
		for col, value := range values {
			cell, _ := excelize.CoordinatesToCellName(col+1, idx)
			_ = file.SetCellValue(sheet, cell, value)
		}
	}

	file.SetColWidth(sheet, "A", "K", 16)

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=worksentry_daily.xlsx")
	_ = file.Write(w)
}
