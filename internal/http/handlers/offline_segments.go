package handlers

import (
	"net/http"
	"strings"
	"time"

	"worksentry/internal/db/sqlc"
)

type OfflineSegmentView struct {
	EmployeeCode string `json:"employeeCode"`
	Name         string `json:"name"`
	Department   string `json:"department"`
	StartAt      string `json:"startAt"`
	EndAt        string `json:"endAt"`
	Duration     string `json:"duration"`
}

func (h *Handler) OfflineSegments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
		return
	}
	dateValue := r.URL.Query().Get("date")
	code := strings.TrimSpace(r.URL.Query().Get("employeeCode"))
	var date time.Time
	var err error
	if dateValue == "" {
		date = time.Now()
	} else {
		date, err = parseDate(dateValue)
		if err != nil {
			writeError(w, http.StatusBadRequest, "日期格式错误")
			return
		}
	}

	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.Local)
	end := start.Add(24 * time.Hour)
	rows, err := h.Queries.ListOfflineSegmentsByDate(r.Context(), sqlc.ListOfflineSegmentsByDateParams{
		StartAt: start,
		EndAt:   end,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取离线段失败")
		return
	}

	views := make([]OfflineSegmentView, 0, len(rows))
	for _, row := range rows {
		if code != "" && row.EmployeeCode != code {
			continue
		}
		views = append(views, OfflineSegmentView{
			EmployeeCode: row.EmployeeCode,
			Name:         row.Name,
			Department:   nullString(row.DepartmentName),
			StartAt:      formatTime(row.StartAt),
			EndAt:        formatTime(row.EndAt),
			Duration:     formatDuration(int64(row.EndAt.Sub(row.StartAt).Seconds())),
		})
	}

	writeJSON(w, http.StatusOK, views)
}
