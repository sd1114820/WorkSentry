package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"worksentry/internal/db/sqlc"
)

type CheckoutTemplatePayload struct {
	ID           int64  `json:"id"`
	DepartmentID int64  `json:"departmentId"`
	Name         string `json:"name"`
	Enabled      bool   `json:"enabled"`
}

type CheckoutTemplateView struct {
	ID           int64  `json:"id"`
	DepartmentID int64  `json:"departmentId"`
	Name         string `json:"name"`
	Enabled      bool   `json:"enabled"`
	FieldCount   int64  `json:"fieldCount"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

type CheckoutFieldPayload struct {
	ID         int64    `json:"id"`
	TemplateID int64    `json:"templateId"`
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Required   bool     `json:"required"`
	SortOrder  int32    `json:"sortOrder"`
	Enabled    bool     `json:"enabled"`
	Options    []string `json:"options"`
}

type CheckoutFieldView struct {
	ID         int64    `json:"id"`
	TemplateID int64    `json:"templateId"`
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Required   bool     `json:"required"`
	SortOrder  int32    `json:"sortOrder"`
	Enabled    bool     `json:"enabled"`
	Options    []string `json:"options"`
}

func (h *Handler) CheckoutTemplates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listCheckoutTemplates(w, r)
	case http.MethodPost:
		h.createCheckoutTemplate(w, r)
	case http.MethodPut:
		h.updateCheckoutTemplate(w, r)
	case http.MethodDelete:
		h.deleteCheckoutTemplate(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
	}
}

func (h *Handler) listCheckoutTemplates(w http.ResponseWriter, r *http.Request) {
	deptID, _ := strconv.ParseInt(r.URL.Query().Get("departmentId"), 10, 64)
	if deptID <= 0 {
		writeJSON(w, http.StatusOK, []CheckoutTemplateView{})
		return
	}

	items, err := h.Queries.ListCheckoutTemplatesByDepartment(r.Context(), deptID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取模板失败")
		return
	}

	views := make([]CheckoutTemplateView, 0, len(items))
	for _, item := range items {
		views = append(views, CheckoutTemplateView{
			ID:           item.ID,
			DepartmentID: item.DepartmentID,
			Name:         item.NameZh,
			Enabled:      item.Enabled,
			FieldCount:   item.FieldCount,
			CreatedAt:    formatTime(item.CreatedAt),
			UpdatedAt:    formatTime(item.UpdatedAt),
		})
	}

	writeJSON(w, http.StatusOK, views)
}

func (h *Handler) createCheckoutTemplate(w http.ResponseWriter, r *http.Request) {
	var payload CheckoutTemplatePayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "参数格式错误")
		return
	}
	payload.Name = strings.TrimSpace(payload.Name)
	if payload.DepartmentID <= 0 {
		writeError(w, http.StatusBadRequest, "请选择部门")
		return
	}
	if payload.Name == "" {
		writeError(w, http.StatusBadRequest, "模板名称不能为空")
		return
	}

	id, err := h.Queries.CreateCheckoutTemplate(r.Context(), sqlc.CreateCheckoutTemplateParams{
		DepartmentID: payload.DepartmentID,
		NameZh:       payload.Name,
		Enabled:      payload.Enabled,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "保存模板失败")
		return
	}

	if payload.Enabled {
		_ = h.Queries.DisableOtherCheckoutTemplates(r.Context(), payload.DepartmentID, id)
	}

	h.logAudit(r, "create_checkout_template", "checkout_template", sql.NullInt64{Int64: id, Valid: true}, payload)
	writeJSON(w, http.StatusOK, map[string]any{"message": "ok", "id": id})
}

func (h *Handler) updateCheckoutTemplate(w http.ResponseWriter, r *http.Request) {
	var payload CheckoutTemplatePayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "参数格式错误")
		return
	}
	payload.Name = strings.TrimSpace(payload.Name)
	if payload.ID <= 0 {
		writeError(w, http.StatusBadRequest, "模板编号不能为空")
		return
	}
	if payload.Name == "" {
		writeError(w, http.StatusBadRequest, "模板名称不能为空")
		return
	}

	template, err := h.Queries.GetCheckoutTemplateByID(r.Context(), payload.ID)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "模板不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取模板失败")
		return
	}

	if err := h.Queries.UpdateCheckoutTemplate(r.Context(), sqlc.UpdateCheckoutTemplateParams{
		ID:      payload.ID,
		NameZh:  payload.Name,
		Enabled: payload.Enabled,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "更新模板失败")
		return
	}

	if payload.Enabled {
		_ = h.Queries.DisableOtherCheckoutTemplates(r.Context(), template.DepartmentID, payload.ID)
	}

	h.logAudit(r, "update_checkout_template", "checkout_template", sql.NullInt64{Int64: payload.ID, Valid: true}, payload)
	writeJSON(w, http.StatusOK, map[string]any{"message": "ok"})
}

func (h *Handler) deleteCheckoutTemplate(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if id <= 0 {
		writeError(w, http.StatusBadRequest, "模板编号无效")
		return
	}

	if err := h.Queries.DeleteCheckoutTemplate(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "删除模板失败")
		return
	}

	h.logAudit(r, "delete_checkout_template", "checkout_template", sql.NullInt64{Int64: id, Valid: true}, nil)
	writeJSON(w, http.StatusOK, map[string]any{"message": "ok"})
}

func (h *Handler) CheckoutFields(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listCheckoutFields(w, r)
	case http.MethodPost:
		h.createCheckoutField(w, r)
	case http.MethodPut:
		h.updateCheckoutField(w, r)
	case http.MethodDelete:
		h.deleteCheckoutField(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
	}
}

func (h *Handler) listCheckoutFields(w http.ResponseWriter, r *http.Request) {
	templateID, _ := strconv.ParseInt(r.URL.Query().Get("templateId"), 10, 64)
	if templateID <= 0 {
		writeError(w, http.StatusBadRequest, "模板编号无效")
		return
	}

	items, err := h.Queries.ListCheckoutFieldsByTemplate(r.Context(), templateID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取字段失败")
		return
	}

	views := make([]CheckoutFieldView, 0, len(items))
	for _, item := range items {
		views = append(views, CheckoutFieldView{
			ID:         item.ID,
			TemplateID: item.TemplateID,
			Name:       item.NameZh,
			Type:       item.Type,
			Required:   item.Required,
			SortOrder:  item.SortOrder,
			Enabled:    item.Enabled,
			Options:    parseOptions(item.OptionsZhJSON),
		})
	}

	writeJSON(w, http.StatusOK, views)
}

func (h *Handler) createCheckoutField(w http.ResponseWriter, r *http.Request) {
	var payload CheckoutFieldPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "参数格式错误")
		return
	}
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Type = strings.TrimSpace(payload.Type)
	payload.Options = normalizeOptions(payload.Options)

	if payload.TemplateID <= 0 {
		writeError(w, http.StatusBadRequest, "模板编号无效")
		return
	}
	if payload.Name == "" {
		writeError(w, http.StatusBadRequest, "字段名称不能为空")
		return
	}
	if !isValidFieldType(payload.Type) {
		writeError(w, http.StatusBadRequest, "字段类型无效")
		return
	}
	if payload.Type == "select" && len(payload.Options) == 0 {
		writeError(w, http.StatusBadRequest, "下拉选项不能为空")
		return
	}

	optionsJSON, err := encodeOptions(payload.Type, payload.Options)
	if err != nil {
		writeError(w, http.StatusBadRequest, "下拉选项格式错误")
		return
	}

	id, err := h.Queries.CreateCheckoutField(r.Context(), sqlc.CreateCheckoutFieldParams{
		TemplateID:    payload.TemplateID,
		NameZh:        payload.Name,
		Type:          payload.Type,
		Required:      payload.Required,
		SortOrder:     payload.SortOrder,
		Enabled:       payload.Enabled,
		OptionsZhJSON: optionsJSON,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "保存字段失败")
		return
	}

	h.logAudit(r, "create_checkout_field", "checkout_field", sql.NullInt64{Int64: id, Valid: true}, payload)
	writeJSON(w, http.StatusOK, map[string]any{"message": "ok", "id": id})
}

func (h *Handler) updateCheckoutField(w http.ResponseWriter, r *http.Request) {
	var payload CheckoutFieldPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "参数格式错误")
		return
	}
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Type = strings.TrimSpace(payload.Type)
	payload.Options = normalizeOptions(payload.Options)

	if payload.ID <= 0 {
		writeError(w, http.StatusBadRequest, "字段编号无效")
		return
	}
	if payload.Name == "" {
		writeError(w, http.StatusBadRequest, "字段名称不能为空")
		return
	}
	if !isValidFieldType(payload.Type) {
		writeError(w, http.StatusBadRequest, "字段类型无效")
		return
	}
	if payload.Type == "select" && len(payload.Options) == 0 {
		writeError(w, http.StatusBadRequest, "下拉选项不能为空")
		return
	}

	optionsJSON, err := encodeOptions(payload.Type, payload.Options)
	if err != nil {
		writeError(w, http.StatusBadRequest, "下拉选项格式错误")
		return
	}

	if err := h.Queries.UpdateCheckoutField(r.Context(), sqlc.UpdateCheckoutFieldParams{
		ID:            payload.ID,
		NameZh:        payload.Name,
		Type:          payload.Type,
		Required:      payload.Required,
		SortOrder:     payload.SortOrder,
		Enabled:       payload.Enabled,
		OptionsZhJSON: optionsJSON,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "更新字段失败")
		return
	}

	h.logAudit(r, "update_checkout_field", "checkout_field", sql.NullInt64{Int64: payload.ID, Valid: true}, payload)
	writeJSON(w, http.StatusOK, map[string]any{"message": "ok"})
}

func (h *Handler) deleteCheckoutField(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if id <= 0 {
		writeError(w, http.StatusBadRequest, "字段编号无效")
		return
	}

	if err := h.Queries.DeleteCheckoutField(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "删除字段失败")
		return
	}

	h.logAudit(r, "delete_checkout_field", "checkout_field", sql.NullInt64{Int64: id, Valid: true}, nil)
	writeJSON(w, http.StatusOK, map[string]any{"message": "ok"})
}

func isValidFieldType(value string) bool {
	switch value {
	case "text", "number", "select":
		return true
	default:
		return false
	}
}

func normalizeOptions(options []string) []string {
	if len(options) == 0 {
		return nil
	}
	cleaned := make([]string, 0, len(options))
	for _, item := range options {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		cleaned = append(cleaned, item)
	}
	return cleaned
}

func encodeOptions(fieldType string, options []string) (sql.NullString, error) {
	if fieldType != "select" || len(options) == 0 {
		return sql.NullString{Valid: false}, nil
	}
	payload, err := json.Marshal(options)
	if err != nil {
		return sql.NullString{}, err
	}
	return sql.NullString{String: string(payload), Valid: true}, nil
}

func parseOptions(value sql.NullString) []string {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}
	var options []string
	if err := json.Unmarshal([]byte(value.String), &options); err != nil {
		return nil
	}
	return normalizeOptions(options)
}
