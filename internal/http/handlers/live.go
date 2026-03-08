package handlers

import (
	"net/http"
	"time"

	"worksentry/internal/db/sqlc"
)

type LiveView struct {
	EmployeeCode  string `json:"employeeCode"`
	Name          string `json:"name"`
	Department    string `json:"department,omitempty"`
	StatusCode    string `json:"statusCode"`
	StatusLabel   string `json:"statusLabel"`
	Description   string `json:"description"`
	LastSeen      string `json:"lastSeen"`
	DelaySeconds  int64  `json:"delaySeconds"`
	Working       bool   `json:"working"`
	HasTodayPunch bool   `json:"hasTodayPunch"`
}

func (h *Handler) LiveSnapshot(w http.ResponseWriter, r *http.Request) {
	items := h.buildLiveSnapshot(r)
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) buildLiveSnapshot(r *http.Request) []LiveView {
	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	dayEnd := dayStart.Add(24 * time.Hour)

	rows, err := h.Queries.ListLiveSnapshot(r.Context(), sqlc.ListLiveSnapshotParams{
		DayStart: dayStart,
		DayEnd:   dayEnd,
	})
	if err != nil {
		return []LiveView{}
	}

	settings := h.getSettingsOrDefault(r)

	items := make([]LiveView, 0, len(rows))
	for _, row := range rows {
		isWorking := row.IsWorking > 0
		status := "offwork"
		delaySeconds := int64(0)
		lastSeen := ""
		if row.LastSeenAt.Valid {
			lastSeen = formatTime(row.LastSeenAt.Time)
			delaySeconds = int64(now.Sub(row.LastSeenAt.Time).Seconds())
		}
		if isWorking {
			status = "offline"
			if row.LastSeenAt.Valid {
				if delaySeconds <= int64(settings.OfflineThresholdSeconds) {
					if row.LastStatus.Valid {
						status = string(row.LastStatus.EmployeesLastStatus)
					} else {
						status = "normal"
					}
				} else {
					status = "offline"
				}
			}
		}
		items = append(items, LiveView{
			EmployeeCode:  row.EmployeeCode,
			Name:          row.Name,
			Department:    nullString(row.DepartmentName),
			StatusCode:    status,
			StatusLabel:   statusLabel(status),
			Description:   nullString(row.LastDescription),
			LastSeen:      lastSeen,
			DelaySeconds:  delaySeconds,
			Working:       isWorking,
			HasTodayPunch: row.HasTodayPunch > 0,
		})
	}
	return items
}
