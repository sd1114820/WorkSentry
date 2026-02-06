package handlers

import (
	"database/sql"
	"net/http"
	"time"
)

type CheckoutTemplateResponse struct {
	Exists   bool                      `json:"exists"`
	Template *CheckoutTemplateSnapshot `json:"template,omitempty"`
}

type CheckoutTemplateSnapshot struct {
	TemplateID int64                   `json:"templateId"`
	Name       string                  `json:"name"`
	Fields     []CheckoutFieldSnapshot `json:"fields"`
}

type CheckoutFieldSnapshot struct {
	ID       int64    `json:"id"`
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Options  []string `json:"options"`
}

func (h *Handler) ClientCheckoutTemplate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
		return
	}

	token := readBearerToken(r)
	if token == "" {
		writeErrorWithCode(w, http.StatusUnauthorized, "token_missing", "缺少令牌")
		return
	}

	clientToken, err := h.Queries.GetToken(r.Context(), token)
	if err == sql.ErrNoRows {
		writeErrorWithCode(w, http.StatusUnauthorized, "token_invalid", "令牌无效")
		return
	}
	if err != nil {
		writeErrorWithCode(w, http.StatusInternalServerError, "token_validation_failed", "令牌校验失败")
		return
	}
	if clientToken.Revoked {
		writeErrorWithCode(w, http.StatusUnauthorized, "token_revoked", "令牌已失效")
		return
	}
	if clientToken.ExpiresAt.Valid && clientToken.ExpiresAt.Time.Before(time.Now()) {
		writeErrorWithCode(w, http.StatusUnauthorized, "token_expired", "令牌已过期")
		return
	}

	employee, err := h.Queries.GetEmployeeByID(r.Context(), clientToken.EmployeeID)
	if err != nil {
		writeErrorWithCode(w, http.StatusInternalServerError, "server_error", "员工不存在")
		return
	}
	if !employee.Enabled {
		writeErrorWithCode(w, http.StatusForbidden, "employee_disabled", "员工已停用")
		return
	}

	if !employee.DepartmentID.Valid || employee.DepartmentID.Int64 <= 0 {
		writeJSON(w, http.StatusOK, CheckoutTemplateResponse{Exists: false})
		return
	}

	template, err := h.Queries.GetEnabledCheckoutTemplateByDepartment(r.Context(), employee.DepartmentID.Int64)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusOK, CheckoutTemplateResponse{Exists: false})
		return
	}
	if err != nil {
		writeErrorWithCode(w, http.StatusInternalServerError, "server_error", "读取模板失败")
		return
	}

	fields, err := h.Queries.ListCheckoutFieldsByTemplate(r.Context(), template.ID)
	if err != nil {
		writeErrorWithCode(w, http.StatusInternalServerError, "server_error", "读取模板字段失败")
		return
	}

	snapshots := make([]CheckoutFieldSnapshot, 0, len(fields))
	for _, field := range fields {
		if !field.Enabled {
			continue
		}
		snapshots = append(snapshots, CheckoutFieldSnapshot{
			ID:       field.ID,
			Name:     field.NameZh,
			Type:     field.Type,
			Required: field.Required,
			Options:  parseOptions(field.OptionsZhJSON),
		})
	}

	writeJSON(w, http.StatusOK, CheckoutTemplateResponse{
		Exists: true,
		Template: &CheckoutTemplateSnapshot{
			TemplateID: template.ID,
			Name:       template.NameZh,
			Fields:     snapshots,
		},
	})
}
