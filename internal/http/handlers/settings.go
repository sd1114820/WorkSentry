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

	err := h.Queries.UpsertSettings(r.Context(), sqlc.UpsertSettingsParams{
		IdleThresholdSeconds:     payload.IdleThresholdSeconds,
		HeartbeatIntervalSeconds: payload.HeartbeatIntervalSeconds,
		OfflineThresholdSeconds:  payload.OfflineThresholdSeconds,
		FishRatioWarnPercent:     payload.FishRatioWarnPercent,
		UpdatePolicy:             int8(payload.UpdatePolicy),
		LatestVersion:            toNullString(payload.LatestVersion),
		UpdateUrl:                toNullString(payload.UpdateURL),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "保存配置失败")
		return
	}

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
	}
}

func (h *Handler) getSettingsOrDefault(r *http.Request) sqlc.Setting {
	return h.getSettingsOrDefaultByContext(r.Context())
}

func (h *Handler) getSettingsOrDefaultByContext(ctx context.Context) sqlc.Setting {
	settings, err := h.Queries.GetSettings(ctx)
	if err != nil {
		return defaultSettings()
	}
	return settings
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
	}
}
