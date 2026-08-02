package handlers

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"worksentry/internal/config"
	"worksentry/internal/db/sqlc"
	"worksentry/internal/mq"
)

type Handler struct {
	Config  *config.Config
	Queries *sqlc.Queries
	Hub     *LiveHub
	DB      *sql.DB
	MQ      mq.Producer

	settingsMu       sync.RWMutex
	settingsCache    sqlc.Setting
	settingsCachedAt time.Time
	settingsCacheOK  bool

	rulesMu       sync.RWMutex
	rulesCache    []sqlc.ListEnabledRulesRow
	rulesCachedAt time.Time
	rulesCacheOK  bool

	historyCleanupMu     sync.RWMutex
	historyCleanupState  HistoryCleanupStatus
	historyCleanupCancel context.CancelFunc

	reportJobs reportJobStore
}

const serverVersion = "20260802-export-download-v6"

func NewHandler(cfg *config.Config, db sqlc.DBTX) *Handler {
	return NewHandlerWithProducer(cfg, db, mq.NewProducer(cfg.MQ))
}

func NewHandlerWithProducer(cfg *config.Config, db sqlc.DBTX, producer mq.Producer) *Handler {
	var sqlDB *sql.DB
	if typed, ok := db.(*sql.DB); ok {
		sqlDB = typed
	}
	if producer == nil {
		producer = mq.DisabledProducer{}
	}
	return &Handler{
		Config:  cfg,
		Queries: sqlc.New(db),
		Hub:     NewLiveHub(),
		DB:      sqlDB,
		MQ:      producer,
	}
}

func (h *Handler) WithLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		w.Header().Set("X-WorkSentry-Version", serverVersion)
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"message": message})
}

func writeErrorWithCode(w http.ResponseWriter, status int, code string, message string) {
	payload := map[string]any{
		"message": message,
	}
	if strings.TrimSpace(code) != "" {
		payload["code"] = strings.TrimSpace(code)
	}
	writeJSON(w, status, payload)
}

func writeErrorWithCodeData(w http.ResponseWriter, status int, code string, message string, data any) {
	payload := map[string]any{
		"message": message,
	}
	if strings.TrimSpace(code) != "" {
		payload["code"] = strings.TrimSpace(code)
	}
	if data != nil {
		payload["data"] = data
	}
	writeJSON(w, status, payload)
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func parseDate(value string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", value, time.Local)
}

func parseDateTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, fmt.Errorf("时间不能为空")
	}
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04",
		"2006-01-02T15:04:05",
	}
	var lastErr error
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return t, nil
		} else {
			lastErr = err
		}
	}
	return time.Time{}, lastErr
}

func formatTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

func formatDuration(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	minutes := seconds / 60
	h := minutes / 60
	m := minutes % 60
	return fmt.Sprintf("%02d:%02d", h, m)
}

func normalizeString(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}

func toNullString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	if value == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: value, Valid: true}
}

func nullString(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

func generateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func newErrorReference() string {
	token, err := generateToken()
	if err == nil && len(token) >= 12 {
		return token[:12]
	}
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

func statusLabel(status string) string {
	switch status {
	case "work":
		return "工作"
	case "normal":
		return "常规"
	case "fish":
		return "摸鱼"
	case "idle":
		return "离开"
	case "break":
		return "休息"
	case "offline":
		return "离线"
	case "offwork":
		return "已下班"
	case "incident":
		return "系统事故"
	default:
		return "未知"
	}
}

func buildDescription(processName, windowTitle string) string {
	processName = strings.TrimSpace(processName)
	windowTitle = strings.TrimSpace(windowTitle)
	processName = strings.TrimSuffix(processName, ".exe")
	if processName == "" {
		return windowTitle
	}
	if windowTitle == "" {
		return processName
	}
	return fmt.Sprintf("%s：%s", processName, windowTitle)
}

func (h *Handler) logAudit(r *http.Request, action string, targetType string, targetID sql.NullInt64, detail any) {
	var payload json.RawMessage
	if detail != nil {
		if b, err := json.Marshal(detail); err == nil {
			payload = json.RawMessage(b)
		}
	}

	operatorID := adminIDFromRequest(r)

	if err := h.Queries.CreateAuditLog(r.Context(), sqlc.CreateAuditLogParams{
		OperatorID: operatorID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Detail:     payload,
	}); err != nil {
		errorID := newErrorReference()
		log.Printf("写入审计日志失败: 错误编号=%s 操作=%s 目标类型=%s 目标编号=%v 错误=%v", errorID, action, targetType, targetID, err)
	}
}
