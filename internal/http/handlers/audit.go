package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"worksentry/internal/db/sqlc"
)

type AuditView struct {
	Action     string `json:"action"`
	TargetType string `json:"targetType"`
	Target     string `json:"target"`
	Detail     string `json:"detail"`
	Operator   string `json:"operator"`
	CreatedAt  string `json:"createdAt"`
}

func (h *Handler) AuditLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
		return
	}
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	startedAt := time.Now()
	var items []sqlc.AuditLog
	var err error
	if date != "" {
		parsed, parseErr := parseDate(date)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "日期格式错误")
			return
		}
		items, err = h.Queries.ListAuditLogsByRange(r.Context(), sqlc.ListAuditLogsByRangeParams{
			CreatedAt:   parsed,
			CreatedAt_2: parsed.Add(24 * time.Hour),
		})
	} else {
		items, err = h.Queries.ListAuditLogs(r.Context())
	}
	if err != nil {
		writeDataQueryError(w, "审计日志", "date="+date, err, startedAt)
		return
	}

	views := make([]AuditView, 0, len(items))
	for _, item := range items {
		detail := ""
		if len(item.Detail) > 0 {
			detail = string(item.Detail)
		}
		if detail != "" {
			var pretty any
			if err := json.Unmarshal([]byte(detail), &pretty); err == nil {
				if b, err := json.Marshal(pretty); err == nil {
					detail = string(b)
				}
			}
		}
		views = append(views, AuditView{
			Action:     item.Action,
			TargetType: item.TargetType,
			Target:     auditTarget(item),
			Detail:     detail,
			Operator:   auditOperator(item.OperatorID),
			CreatedAt:  formatTime(item.CreatedAt),
		})
	}

	writeJSON(w, http.StatusOK, views)
}

func auditTarget(item sqlc.AuditLog) string {
	if item.TargetID.Valid {
		return fmt.Sprintf("%s #%d", item.TargetType, item.TargetID.Int64)
	}
	return item.TargetType
}

func auditOperator(operatorID int64) string {
	if operatorID == 0 {
		return "系统"
	}
	return "管理员"
}
