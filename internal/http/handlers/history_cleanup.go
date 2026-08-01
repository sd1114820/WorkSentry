package handlers

import (
	"database/sql"
	"net/http"
)

type HistoryCleanupRequest struct {
	RetentionDays int `json:"retentionDays"`
}

func (h *Handler) HistoryCleanup(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getHistoryCleanup(w, r)
	case http.MethodPost:
		h.startManualHistoryCleanup(w, r)
	case http.MethodDelete:
		h.stopManualHistoryCleanup(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
	}
}

func (h *Handler) stopManualHistoryCleanup(w http.ResponseWriter, r *http.Request) {
	if !h.cancelHistoryCleanup() {
		writeError(w, http.StatusConflict, "当前没有正在运行的历史数据清理")
		return
	}
	h.logAudit(r, "cancel_history_cleanup", "settings", sql.NullInt64{}, nil)
	writeJSON(w, http.StatusAccepted, h.getHistoryCleanupStatus())
}

func (h *Handler) getHistoryCleanup(w http.ResponseWriter, r *http.Request) {
	settings := h.getSettingsOrDefault(r)
	status := h.getHistoryCleanupStatus()
	if status.Message == "" {
		status.Message = "尚未执行历史数据清理"
	}
	if status.RetentionDays == 0 {
		status.RetentionDays = int(settings.HistoryRetentionDays)
	}
	if status.LastRunAt == "" && settings.HistoryCleanupLastRunAt.Valid {
		status.LastRunAt = formatTime(settings.HistoryCleanupLastRunAt.Time)
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) startManualHistoryCleanup(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		writeError(w, http.StatusInternalServerError, "数据库未初始化")
		return
	}
	var payload HistoryCleanupRequest
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "参数格式错误")
		return
	}
	retentionDays := payload.RetentionDays
	if retentionDays < 1 || retentionDays > 3650 {
		writeError(w, http.StatusBadRequest, "历史数据保留天数必须在 1-3650 之间")
		return
	}
	if !h.startHistoryCleanup(r.Context(), "手动", retentionDays) {
		if h.getHistoryCleanupStatus().Running {
			writeError(w, http.StatusConflict, "历史数据清理正在进行，请勿重复执行")
			return
		}
		writeError(w, http.StatusInternalServerError, "无法启动历史数据清理")
		return
	}

	h.logAudit(r, "start_history_cleanup", "settings", sql.NullInt64{}, map[string]any{
		"retentionDays": retentionDays,
	})
	writeJSON(w, http.StatusAccepted, h.getHistoryCleanupStatus())
}
