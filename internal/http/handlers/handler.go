package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"worksentry/internal/config"
	"worksentry/internal/db/sqlc"
)

type Handler struct {
	Config  *config.Config
	Queries *sqlc.Queries
	Hub     *LiveHub
}

func NewHandler(cfg *config.Config, db sqlc.DBTX) *Handler {
	return &Handler{
		Config:  cfg,
		Queries: sqlc.New(db),
		Hub:     NewLiveHub(),
	}
}

func (h *Handler) WithLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
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

	_ = h.Queries.CreateAuditLog(r.Context(), sqlc.CreateAuditLogParams{
		OperatorID: operatorID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Detail:     payload,
	})
}
