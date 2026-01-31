package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"worksentry/internal/db/sqlc"
)

type RulePayload struct {
	ID         int64  `json:"id"`
	RuleType   string `json:"type"`
	MatchMode  string `json:"matchMode"`
	MatchValue string `json:"matchValue"`
	Enabled    bool   `json:"enabled"`
	Remark     string `json:"remark"`
}

type RuleView struct {
	ID             int64  `json:"id"`
	RuleType       string `json:"type"`
	RuleTypeLabel  string `json:"typeLabel"`
	MatchMode      string `json:"matchMode"`
	MatchModeLabel string `json:"matchModeLabel"`
	MatchValue     string `json:"matchValue"`
	Enabled        bool   `json:"enabled"`
	Remark         string `json:"remark"`
	CreatedAt      string `json:"createdAt"`
}

func (h *Handler) Rules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listRules(w, r)
	case http.MethodPost:
		h.createRule(w, r)
	case http.MethodPut:
		h.updateRule(w, r)
	case http.MethodDelete:
		h.deleteRule(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
	}
}

func (h *Handler) listRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.Queries.ListRules(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取规则失败")
		return
	}

	views := make([]RuleView, 0, len(rules))
	for _, rule := range rules {
		ruleType := string(rule.RuleType)
		matchMode := string(rule.MatchMode)
		views = append(views, RuleView{
			ID:             rule.ID,
			RuleType:       ruleType,
			RuleTypeLabel:  ruleTypeLabel(ruleType),
			MatchMode:      matchMode,
			MatchModeLabel: matchModeLabel(matchMode),
			MatchValue:     rule.MatchValue,
			Enabled:        rule.Enabled,
			Remark:         nullString(rule.Remark),
			CreatedAt:      rule.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	writeJSON(w, http.StatusOK, views)
}

func (h *Handler) createRule(w http.ResponseWriter, r *http.Request) {
	var payload RulePayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "参数格式错误")
		return
	}

	payload.RuleType = strings.TrimSpace(strings.ToLower(payload.RuleType))
	payload.MatchMode = strings.TrimSpace(strings.ToLower(payload.MatchMode))
	payload.MatchValue = strings.TrimSpace(payload.MatchValue)

	if err := validateRule(payload); err != "" {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	result, err := h.Queries.CreateRule(r.Context(), sqlc.CreateRuleParams{
		RuleType:   sqlc.RulesRuleType(payload.RuleType),
		MatchMode:  sqlc.RulesMatchMode(payload.MatchMode),
		MatchValue: payload.MatchValue,
		Enabled:    payload.Enabled,
		Remark:     toNullString(payload.Remark),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "保存规则失败")
		return
	}

	id, err := result.LastInsertId()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "保存规则失败")
		return
	}

	h.logAudit(r, "create_rule", "rule", sql.NullInt64{Int64: id, Valid: true}, payload)
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}
func (h *Handler) updateRule(w http.ResponseWriter, r *http.Request) {
	var payload RulePayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "参数格式错误")
		return
	}

	if payload.ID <= 0 {
		writeError(w, http.StatusBadRequest, "规则编号不能为空")
		return
	}
	payload.RuleType = strings.TrimSpace(strings.ToLower(payload.RuleType))
	payload.MatchMode = strings.TrimSpace(strings.ToLower(payload.MatchMode))
	payload.MatchValue = strings.TrimSpace(payload.MatchValue)

	if err := validateRule(payload); err != "" {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	err := h.Queries.UpdateRule(r.Context(), sqlc.UpdateRuleParams{
		ID:         payload.ID,
		RuleType:   sqlc.RulesRuleType(payload.RuleType),
		MatchMode:  sqlc.RulesMatchMode(payload.MatchMode),
		MatchValue: strings.TrimSpace(payload.MatchValue),
		Enabled:    payload.Enabled,
		Remark:     toNullString(payload.Remark),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "更新规则失败")
		return
	}

	h.logAudit(r, "update_rule", "rule", sql.NullInt64{Int64: payload.ID, Valid: true}, payload)
	writeJSON(w, http.StatusOK, map[string]string{"message": "更新成功"})
}

func (h *Handler) deleteRule(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "规则编号无效")
		return
	}

	if err := h.Queries.DeleteRule(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "删除规则失败")
		return
	}

	h.logAudit(r, "delete_rule", "rule", sql.NullInt64{Int64: id, Valid: true}, nil)
	writeJSON(w, http.StatusOK, map[string]string{"message": "删除成功"})
}

func ruleTypeLabel(value string) string {
	switch value {
	case "white":
		return "白名单"
	case "black":
		return "黑名单"
	default:
		return "未知"
	}
}

func matchModeLabel(value string) string {
	switch value {
	case "process":
		return "进程名"
	case "title":
		return "标题关键词"
	default:
		return "未知"
	}
}

func validateRule(payload RulePayload) string {
	payload.RuleType = strings.TrimSpace(payload.RuleType)
	payload.MatchMode = strings.TrimSpace(payload.MatchMode)
	payload.MatchValue = strings.TrimSpace(payload.MatchValue)

	if payload.RuleType != "white" && payload.RuleType != "black" {
		return "规则类型必须是白名单或黑名单"
	}
	if payload.MatchMode != "process" && payload.MatchMode != "title" {
		return "匹配方式必须是进程名或标题关键词"
	}
	if payload.MatchValue == "" {
		return "匹配值不能为空"
	}
	return ""
}
