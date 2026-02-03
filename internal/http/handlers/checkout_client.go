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
		writeError(w, http.StatusUnauthorized, "缺少令牌")
		return
	}

	clientToken, err := h.Queries.GetToken(r.Context(), token)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusUnauthorized, "令牌无效")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "令牌校验失败")
		return
	}
	if clientToken.Revoked {
		writeError(w, http.StatusUnauthorized, "令牌已失效")
		return
	}
	if clientToken.ExpiresAt.Valid && clientToken.ExpiresAt.Time.Before(time.Now()) {
		writeError(w, http.StatusUnauthorized, "令牌已过期")
		return
	}

	employee, err := h.Queries.GetEmployeeByID(r.Context(), clientToken.EmployeeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "员工不存在")
		return
	}
	if !employee.Enabled {
		writeError(w, http.StatusForbidden, "员工已停用")
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
		writeError(w, http.StatusInternalServerError, "读取模板失败")
		return
	}

	fields, err := h.Queries.ListCheckoutFieldsByTemplate(r.Context(), template.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取模板字段失败")
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
