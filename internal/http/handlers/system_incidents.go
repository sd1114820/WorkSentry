package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"worksentry/internal/db/sqlc"
)

type IncidentPayload struct {
	ID      int64  `json:"id"`
	StartAt string `json:"startAt"`
	EndAt   string `json:"endAt"`
	Reason  string `json:"reason"`
	Note    string `json:"note"`
}

type IncidentView struct {
	ID      int64  `json:"id"`
	StartAt string `json:"startAt"`
	EndAt   string `json:"endAt"`
	Reason  string `json:"reason"`
	Note    string `json:"note"`
	Created string `json:"createdAt"`
}

func (h *Handler) SystemIncidents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listIncidents(w, r)
	case http.MethodPost:
		h.createIncident(w, r)
	case http.MethodPut:
		h.updateIncident(w, r)
	case http.MethodDelete:
		h.deleteIncident(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
	}
}

func (h *Handler) listIncidents(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	params := sqlc.ListIncidentsParams{
		Column1: date,
		StartAt: time.Time{},
	}
	if date != "" {
		parsed, err := parseDate(date)
		if err != nil {
			writeError(w, http.StatusBadRequest, "日期格式错误")
			return
		}
		params.StartAt = parsed
	}
	items, err := h.Queries.ListIncidents(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取事故失败")
		return
	}

	views := make([]IncidentView, 0, len(items))
	for _, item := range items {
		views = append(views, IncidentView{
			ID:      item.ID,
			StartAt: formatTime(item.StartAt),
			EndAt:   formatTime(item.EndAt),
			Reason:  item.Reason,
			Note:    nullString(item.Note),
			Created: formatTime(item.CreatedAt),
		})
	}

	writeJSON(w, http.StatusOK, views)
}

func (h *Handler) createIncident(w http.ResponseWriter, r *http.Request) {
	var payload IncidentPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "参数格式错误")
		return
	}
	startAt, err := parseDateTime(payload.StartAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "开始时间格式错误")
		return
	}
	endAt, err := parseDateTime(payload.EndAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "结束时间格式错误")
		return
	}
	if !endAt.After(startAt) {
		writeError(w, http.StatusBadRequest, "结束时间必须大于开始时间")
		return
	}
	if payload.Reason == "" {
		writeError(w, http.StatusBadRequest, "原因不能为空")
		return
	}

	result, err := h.Queries.CreateIncident(r.Context(), sqlc.CreateIncidentParams{
		StartAt: startAt,
		EndAt:   endAt,
		Reason:  payload.Reason,
		Note:    toNullString(payload.Note),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "创建事故失败")
		return
	}

	id, err := result.LastInsertId()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "创建事故失败")
		return
	}

	h.logAudit(r, "create_incident", "incident", sql.NullInt64{Int64: id, Valid: true}, payload)
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}
func (h *Handler) updateIncident(w http.ResponseWriter, r *http.Request) {
	var payload IncidentPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "参数格式错误")
		return
	}
	if payload.ID <= 0 {
		writeError(w, http.StatusBadRequest, "事故编号不能为空")
		return
	}
	startAt, err := parseDateTime(payload.StartAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "开始时间格式错误")
		return
	}
	endAt, err := parseDateTime(payload.EndAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "结束时间格式错误")
		return
	}
	if !endAt.After(startAt) {
		writeError(w, http.StatusBadRequest, "结束时间必须大于开始时间")
		return
	}

	if err := h.Queries.UpdateIncident(r.Context(), sqlc.UpdateIncidentParams{
		ID:      payload.ID,
		StartAt: startAt,
		EndAt:   endAt,
		Reason:  payload.Reason,
		Note:    toNullString(payload.Note),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "更新事故失败")
		return
	}

	h.logAudit(r, "update_incident", "incident", sql.NullInt64{Int64: payload.ID, Valid: true}, payload)
	writeJSON(w, http.StatusOK, map[string]string{"message": "更新成功"})
}

func (h *Handler) deleteIncident(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "事故编号无效")
		return
	}

	if err := h.Queries.DeleteIncident(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "删除事故失败")
		return
	}

	h.logAudit(r, "delete_incident", "incident", sql.NullInt64{Int64: id, Valid: true}, nil)
	writeJSON(w, http.StatusOK, map[string]string{"message": "删除成功"})
}
