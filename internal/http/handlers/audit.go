package handlers

import (
	"encoding/json"
	"net/http"
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
	date := r.URL.Query().Get("date")
	params := sqlc.ListAuditLogsParams{
		Column1:   date,
		CreatedAt: time.Time{},
	}
	if date != "" {
		parsed, err := parseDate(date)
		if err != nil {
			writeError(w, http.StatusBadRequest, "日期格式错误")
			return
		}
		params.CreatedAt = parsed
	}
	items, err := h.Queries.ListAuditLogs(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取审计失败")
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
		return item.TargetType
	}
	return item.TargetType
}

func auditOperator(operatorID int64) string {
	if operatorID == 0 {
		return "系统"
	}
	return "管理员"
}
