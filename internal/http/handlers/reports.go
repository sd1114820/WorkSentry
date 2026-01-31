package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"worksentry/internal/db/sqlc"
)

type DailyReportView struct {
	EmployeeCode       string `json:"employeeCode"`
	Name               string `json:"name"`
	Department         string `json:"department"`
	WorkDuration       string `json:"workDuration"`
	NormalDuration     string `json:"normalDuration"`
	FishDuration       string `json:"fishDuration"`
	IdleDuration       string `json:"idleDuration"`
	OfflineDuration    string `json:"offlineDuration"`
	AttendanceDuration string `json:"attendanceDuration"`
	EffectiveDuration  string `json:"effectiveDuration"`
}

type DailyReportResponse struct {
	Date  string            `json:"date"`
	Items []DailyReportView `json:"items"`
}

type TimelineItem struct {
	StatusLabel string `json:"statusLabel"`
	StatusCode  string `json:"statusCode"`
	StartAt     string `json:"startAt"`
	EndAt       string `json:"endAt"`
	Duration    string `json:"duration"`
	Description string `json:"description"`
	SourceLabel string `json:"sourceLabel"`
}

type RankItem struct {
	EmployeeCode string `json:"employeeCode"`
	Name         string `json:"name"`
	Department   string `json:"department"`
	Value        string `json:"value"`
}

type RankResponse struct {
	Date    string     `json:"date"`
	WorkTop []RankItem `json:"workTop"`
	FishTop []RankItem `json:"fishTop"`
}

type rankValue struct {
	Item  RankItem
	Score float64
}

func (h *Handler) ReportDaily(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
		return
	}
	dateValue := r.URL.Query().Get("date")
	departmentID := parseInt64(r.URL.Query().Get("departmentId"))
	if dateValue == "" {
		dateValue = time.Now().Format("2006-01-02")
	}
	date, err := parseDate(dateValue)
	if err != nil {
		writeError(w, http.StatusBadRequest, "日期格式错误")
		return
	}
	rows, err := h.Queries.ListDailyStatsByDate(r.Context(), sqlc.ListDailyStatsByDateParams{
		StatDate:     date,
		Column2:      departmentID,
		DepartmentID: toNullInt64(departmentID),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取报表失败")
		return
	}

	items := make([]DailyReportView, 0, len(rows))
	for _, row := range rows {
		items = append(items, DailyReportView{
			EmployeeCode:       row.EmployeeCode,
			Name:               row.Name,
			Department:         nullString(row.DepartmentName),
			WorkDuration:       formatDuration(int64(row.WorkSeconds)),
			NormalDuration:     formatDuration(int64(row.NormalSeconds)),
			FishDuration:       formatDuration(int64(row.FishSeconds)),
			IdleDuration:       formatDuration(int64(row.IdleSeconds)),
			OfflineDuration:    formatDuration(int64(row.OfflineSeconds)),
			AttendanceDuration: formatDuration(int64(row.AttendanceSeconds)),
			EffectiveDuration:  formatDuration(int64(row.EffectiveSeconds)),
		})
	}

	writeJSON(w, http.StatusOK, DailyReportResponse{
		Date:  date.Format("2006-01-02"),
		Items: items,
	})
}

func (h *Handler) ReportTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
		return
	}
	code := r.URL.Query().Get("employeeCode")
	dateValue := r.URL.Query().Get("date")
	if code == "" || dateValue == "" {
		writeError(w, http.StatusBadRequest, "工号与日期不能为空")
		return
	}
	date, err := parseDate(dateValue)
	if err != nil {
		writeError(w, http.StatusBadRequest, "日期格式错误")
		return
	}

	employee, err := h.Queries.GetEmployeeByCode(r.Context(), code)
	if err != nil {
		writeError(w, http.StatusNotFound, "员工不存在")
		return
	}

	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.Local)
	end := start.Add(24 * time.Hour)
	segments, err := h.Queries.ListTimeSegmentsByEmployeeAndRange(r.Context(), sqlc.ListTimeSegmentsByEmployeeAndRangeParams{
		EmployeeID: employee.ID,
		StartAt:    end,
		EndAt:      start,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取时间轴失败")
		return
	}

	items := make([]TimelineItem, 0, len(segments))
	for _, seg := range segments {
		statusCode := string(seg.Status)
		items = append(items, TimelineItem{
			StatusLabel: statusLabel(statusCode),
			StatusCode:  statusCode,
			StartAt:     formatTime(seg.StartAt),
			EndAt:       formatTime(seg.EndAt),
			Duration:    formatDuration(int64(seg.EndAt.Sub(seg.StartAt).Seconds())),
			Description: nullString(seg.Description),
			SourceLabel: sourceLabel(string(seg.Source)),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"employee": map[string]string{"code": employee.EmployeeCode, "name": employee.Name},
		"date":     date.Format("2006-01-02"),
		"items":    items,
	})
}

func (h *Handler) ReportRank(w http.ResponseWriter, r *http.Request) {
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
	rows, err := h.Queries.ListDailyStatsByDate(r.Context(), sqlc.ListDailyStatsByDateParams{
		StatDate:     date,
		Column2:      int64(0),
		DepartmentID: toNullInt64(0),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取排行失败")
		return
	}

	workList := make([]rankValue, 0, len(rows))
	fishList := make([]rankValue, 0, len(rows))
	for _, row := range rows {
		workList = append(workList, rankValue{
			Item: RankItem{
				EmployeeCode: row.EmployeeCode,
				Name:         row.Name,
				Department:   nullString(row.DepartmentName),
				Value:        formatDuration(int64(row.EffectiveSeconds)),
			},
			Score: float64(row.EffectiveSeconds),
		})
		fishRatio := 0.0
		if row.AttendanceSeconds > 0 {
			fishRatio = float64(row.FishSeconds) / float64(row.AttendanceSeconds)
		}
		fishList = append(fishList, rankValue{
			Item: RankItem{
				EmployeeCode: row.EmployeeCode,
				Name:         row.Name,
				Department:   nullString(row.DepartmentName),
				Value:        formatPercent(fishRatio),
			},
			Score: fishRatio,
		})
	}

	sort.Slice(workList, func(i, j int) bool { return workList[i].Score > workList[j].Score })
	sort.Slice(fishList, func(i, j int) bool { return fishList[i].Score > fishList[j].Score })

	workTop := make([]RankItem, 0, minInt(len(workList), 10))
	for i := 0; i < len(workList) && i < 10; i++ {
		workTop = append(workTop, workList[i].Item)
	}
	fishTop := make([]RankItem, 0, minInt(len(fishList), 10))
	for i := 0; i < len(fishList) && i < 10; i++ {
		fishTop = append(fishTop, fishList[i].Item)
	}

	writeJSON(w, http.StatusOK, RankResponse{
		Date:    dateValue,
		WorkTop: workTop,
		FishTop: fishTop,
	})
}

func sourceLabel(value string) string {
	switch value {
	case "system":
		return "系统"
	case "offline":
		return "离线"
	case "manual":
		return "补录"
	case "incident":
		return "系统事故"
	default:
		return "未知"
	}
}

func formatPercent(value float64) string {
	return fmt.Sprintf("%.1f%%", value*100)
}

func parseInt64(value string) int64 {
	if value == "" {
		return 0
	}
	n, _ := strconv.ParseInt(value, 10, 64)
	return n
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
