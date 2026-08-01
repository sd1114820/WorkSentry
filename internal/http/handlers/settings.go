package handlers

import (
	"context"
	"database/sql"
	"net/http"

	"worksentry/internal/db/sqlc"
)

type SettingsPayload struct {
	IdleThresholdSeconds     int32  `json:"idleThresholdSeconds"`
	HeartbeatIntervalSeconds int32  `json:"heartbeatIntervalSeconds"`
	OfflineThresholdSeconds  int32  `json:"offlineThresholdSeconds"`
	FishRatioWarnPercent     int32  `json:"fishRatioWarnPercent"`
	UpdatePolicy             int32  `json:"updatePolicy"`
	LatestVersion            string `json:"latestVersion"`
	UpdateURL                string `json:"updateUrl"`
	HistoryCleanupEnabled    bool   `json:"historyCleanupEnabled"`
	HistoryRetentionDays     int32  `json:"historyRetentionDays"`
	HistoryCleanupHour       int32  `json:"historyCleanupHour"`
	HistoryCleanupLastRunAt  string `json:"historyCleanupLastRunAt,omitempty"`
}

func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getSettings(w, r)
	case http.MethodPut:
		h.updateSettings(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
	}
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.Queries.GetSettings(r.Context())
	if err == sql.ErrNoRows {
		defaults := defaultSettings()
		writeJSON(w, http.StatusOK, h.settingsView(defaults))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取配置失败")
		return
	}

	writeJSON(w, http.StatusOK, h.settingsView(settings))
}

func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	var payload SettingsPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "参数格式错误")
		return
	}

	if payload.IdleThresholdSeconds <= 0 || payload.HeartbeatIntervalSeconds <= 0 || payload.OfflineThresholdSeconds <= 0 {
		writeError(w, http.StatusBadRequest, "阈值必须大于 0")
		return
	}

	if payload.FishRatioWarnPercent < 0 || payload.FishRatioWarnPercent > 100 {
		writeError(w, http.StatusBadRequest, "摸鱼比例阈值范围 0-100")
		return
	}

	if payload.UpdatePolicy < 0 || payload.UpdatePolicy > 1 {
		writeError(w, http.StatusBadRequest, "更新策略仅支持 0 或 1")
		return
	}
	if payload.HistoryRetentionDays < 1 || payload.HistoryRetentionDays > 3650 {
		writeError(w, http.StatusBadRequest, "历史数据保留天数必须在 1-3650 之间")
		return
	}
	if payload.HistoryCleanupHour < 0 || payload.HistoryCleanupHour > 23 {
		writeError(w, http.StatusBadRequest, "历史数据清理小时必须在 0-23 之间")
		return
	}

	err := h.Queries.UpsertSettings(r.Context(), sqlc.UpsertSettingsParams{
		IdleThresholdSeconds:     payload.IdleThresholdSeconds,
		HeartbeatIntervalSeconds: payload.HeartbeatIntervalSeconds,
		OfflineThresholdSeconds:  payload.OfflineThresholdSeconds,
		FishRatioWarnPercent:     payload.FishRatioWarnPercent,
		UpdatePolicy:             int8(payload.UpdatePolicy),
		LatestVersion:            toNullString(payload.LatestVersion),
		UpdateUrl:                toNullString(payload.UpdateURL),
		HistoryCleanupEnabled:    boolToInt8(payload.HistoryCleanupEnabled),
		HistoryRetentionDays:     payload.HistoryRetentionDays,
		HistoryCleanupHour:       int8(payload.HistoryCleanupHour),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "保存配置失败")
		return
	}

	h.invalidateSettingsCache()

	h.logAudit(r, "update_settings", "settings", sql.NullInt64{}, payload)
	writeJSON(w, http.StatusOK, map[string]string{"message": "保存成功"})
}

func (h *Handler) settingsView(settings sqlc.Setting) SettingsPayload {
	return SettingsPayload{
		IdleThresholdSeconds:     settings.IdleThresholdSeconds,
		HeartbeatIntervalSeconds: settings.HeartbeatIntervalSeconds,
		OfflineThresholdSeconds:  settings.OfflineThresholdSeconds,
		FishRatioWarnPercent:     settings.FishRatioWarnPercent,
		UpdatePolicy:             int32(settings.UpdatePolicy),
		LatestVersion:            nullString(settings.LatestVersion),
		UpdateURL:                nullString(settings.UpdateUrl),
		HistoryCleanupEnabled:    settings.HistoryCleanupEnabled > 0,
		HistoryRetentionDays:     settings.HistoryRetentionDays,
		HistoryCleanupHour:       int32(settings.HistoryCleanupHour),
		HistoryCleanupLastRunAt:  formatNullTime(settings.HistoryCleanupLastRunAt),
	}
}

func (h *Handler) getSettingsOrDefault(r *http.Request) sqlc.Setting {
	return h.getSettingsOrDefaultByContext(r.Context())
}

func (h *Handler) getSettingsOrDefaultByContext(ctx context.Context) sqlc.Setting {
	return h.getSettingsCached(ctx)
}

func defaultSettings() sqlc.Setting {
	return sqlc.Setting{
		ID:                       1,
		IdleThresholdSeconds:     300,
		HeartbeatIntervalSeconds: 300,
		OfflineThresholdSeconds:  600,
		FishRatioWarnPercent:     10,
		UpdatePolicy:             0,
		LatestVersion:            sql.NullString{},
		UpdateUrl:                sql.NullString{},
		HistoryCleanupEnabled:    0,
		HistoryRetentionDays:     40,
		HistoryCleanupHour:       3,
		HistoryCleanupLastRunAt:  sql.NullTime{},
	}
}

func boolToInt8(value bool) int8 {
	if value {
		return 1
	}
	return 0
}

func formatNullTime(value sql.NullTime) string {
	if !value.Valid {
		return ""
	}
	return formatTime(value.Time)
}
