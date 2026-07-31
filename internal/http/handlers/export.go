package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/xuri/excelize/v2"
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
	rows, err := h.Queries.ListDailyStatsForExportByDate(r.Context(), buildDailyStatsExportParams(date, departmentID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "导出失败")
		return
	}

	file := excelize.NewFile()
	sheet := "日报表"
	file.SetSheetName("Sheet1", sheet)

	headers := []string{"日期", "工号", "姓名", "部门", "上班打卡时间", "下班打卡时间", "上班时长", "工作时长", "常规时长", "摸鱼时长", "离开时长", "离线时长", "在岗时长", "有效工时"}
	for col, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		_ = file.SetCellValue(sheet, cell, header)
	}

	for i, row := range rows {
		idx := i + 2
		metrics := buildDailyReportMetrics(
			int64(row.NormalSeconds),
			int64(row.FishSeconds),
			int64(row.IdleSeconds),
			int64(row.OfflineSeconds),
			int64(row.EffectiveSeconds),
			row.BreakSeconds,
			row.OnDutySeconds,
		)
		values := []any{
			dateValue,
			row.EmployeeCode,
			row.Name,
			nullString(row.DepartmentName),
			formatPunchHHmmOrMissing(interfaceToNullTime(row.FirstStartAt)),
			formatPunchHHmmOrMissing(interfaceToNullTime(row.LastEndAt)),
			formatDuration(metrics.OnDutySeconds),
			formatDuration(metrics.WorkSeconds),
			formatDuration(int64(row.NormalSeconds)),
			formatDuration(int64(row.FishSeconds)),
			formatDuration(metrics.IdleSeconds),
			formatDuration(int64(row.OfflineSeconds)),
			formatDuration(int64(row.AttendanceSeconds)),
			formatDuration(int64(row.EffectiveSeconds)),
		}
		for col, value := range values {
			cell, _ := excelize.CoordinatesToCellName(col+1, idx)
			_ = file.SetCellValue(sheet, cell, value)
		}
	}

	file.SetColWidth(sheet, "A", "N", 16)

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=worksentry_daily.xlsx")
	_ = file.Write(w)
}

func (h *Handler) ExportEmployees(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
		return
	}

	enabledOnly := true
	enabledOnlyRaw := r.URL.Query().Get("enabledOnly")
	if enabledOnlyRaw != "" {
		parsed, parseErr := strconv.ParseBool(enabledOnlyRaw)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "导出参数错误")
			return
		}
		enabledOnly = parsed
	}

	rows, err := h.Queries.ListEmployeesAdmin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "导出失败")
		return
	}

	file := excelize.NewFile()
	sheet := "员工台账"
	file.SetSheetName("Sheet1", sheet)

	headers := []string{"工号", "姓名", "部门", "绑定状态", "上班时间", "下班时间", "最近上报", "状态"}
	for col, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		_ = file.SetCellValue(sheet, cell, header)
	}

	seenEmployees := make(map[int64]struct{}, len(rows))
	excelRow := 2
	for _, row := range rows {
		if enabledOnly && !row.Enabled {
			continue
		}
		if _, exists := seenEmployees[row.ID]; exists {
			continue
		}
		seenEmployees[row.ID] = struct{}{}

		clockIn := ""
		lastStartAt := interfaceToNullTime(row.LastStartAt)
		if lastStartAt.Valid {
			clockIn = formatTime(lastStartAt.Time)
		}
		clockOut := ""
		lastEndAt := interfaceToNullTime(row.LastEndAt)
		if lastEndAt.Valid {
			clockOut = formatTime(lastEndAt.Time)
		}

		bindStatus := "未绑定"
		if row.FingerprintHash.Valid {
			bindStatus = "已绑定"
		}

		statusLabel := "停用"
		if row.Enabled {
			statusLabel = "启用"
		}

		values := []any{
			row.EmployeeCode,
			row.Name,
			nullString(row.DepartmentName),
			bindStatus,
			formatEmployeeClockIn(clockIn),
			formatEmployeeClockOut(clockIn, clockOut),
			formatEmployeeRecentSeen(row.LastSeenAt),
			statusLabel,
		}
		for col, value := range values {
			cell, _ := excelize.CoordinatesToCellName(col+1, excelRow)
			_ = file.SetCellValue(sheet, cell, value)
		}
		excelRow++
	}

	file.SetColWidth(sheet, "A", "H", 18)

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=worksentry_employees.xlsx")
	_ = file.Write(w)
}

func formatEmployeeClockIn(value string) string {
	if value == "" {
		return "未上班"
	}
	return value
}

func formatEmployeeClockOut(clockIn string, clockOut string) string {
	if clockOut != "" {
		return clockOut
	}
	if clockIn != "" {
		return "未下班"
	}
	return "-"
}

func formatEmployeeRecentSeen(value sql.NullTime) string {
	if !value.Valid {
		return "-"
	}
	return formatTime(value.Time)
}

func formatPunchHHmmOrMissing(value sql.NullTime) string {
	if !value.Valid {
		return "未打卡"
	}
	return value.Time.In(time.Local).Format("15:04")
}
