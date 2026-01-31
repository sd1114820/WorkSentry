package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"worksentry/internal/db/sqlc"
)

type DepartmentPayload struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	ParentID int64  `json:"parentId"`
}

type DepartmentView struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	ParentID   int64  `json:"parentId"`
	ParentName string `json:"parentName"`
}

func (h *Handler) Departments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listDepartments(w, r)
	case http.MethodPost:
		h.createDepartment(w, r)
	case http.MethodPut:
		h.updateDepartment(w, r)
	case http.MethodDelete:
		h.deleteDepartment(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
	}
}

func (h *Handler) listDepartments(w http.ResponseWriter, r *http.Request) {
	items, err := h.Queries.ListDepartments(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取部门失败")
		return
	}

	nameMap := make(map[int64]string)
	for _, item := range items {
		nameMap[item.ID] = item.Name
	}

	views := make([]DepartmentView, 0, len(items))
	for _, item := range items {
		parentID := int64(0)
		parentName := ""
		if item.ParentID.Valid {
			parentID = item.ParentID.Int64
			parentName = nameMap[parentID]
		}
		views = append(views, DepartmentView{
			ID:         item.ID,
			Name:       item.Name,
			ParentID:   parentID,
			ParentName: parentName,
		})
	}

	writeJSON(w, http.StatusOK, views)
}

func (h *Handler) createDepartment(w http.ResponseWriter, r *http.Request) {
	var payload DepartmentPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "参数格式错误")
		return
	}
	payload.Name = strings.TrimSpace(payload.Name)
	if payload.Name == "" {
		writeError(w, http.StatusBadRequest, "部门名称不能为空")
		return
	}

	if err := h.Queries.CreateDepartment(r.Context(), sqlc.CreateDepartmentParams{
		Name:     payload.Name,
		ParentID: toNullInt64(payload.ParentID),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "新增部门失败")
		return
	}

	h.logAudit(r, "create_department", "department", sql.NullInt64{}, payload)
	writeJSON(w, http.StatusOK, map[string]string{"message": "保存成功"})
}

func (h *Handler) updateDepartment(w http.ResponseWriter, r *http.Request) {
	var payload DepartmentPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "参数格式错误")
		return
	}
	payload.Name = strings.TrimSpace(payload.Name)
	if payload.ID <= 0 {
		writeError(w, http.StatusBadRequest, "部门编号不能为空")
		return
	}
	if payload.Name == "" {
		writeError(w, http.StatusBadRequest, "部门名称不能为空")
		return
	}
	if payload.ParentID == payload.ID {
		writeError(w, http.StatusBadRequest, "上级部门不能是自己")
		return
	}

	if err := h.Queries.UpdateDepartment(r.Context(), sqlc.UpdateDepartmentParams{
		ID:       payload.ID,
		Name:     payload.Name,
		ParentID: toNullInt64(payload.ParentID),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "更新部门失败")
		return
	}

	h.logAudit(r, "update_department", "department", sql.NullInt64{Int64: payload.ID, Valid: true}, payload)
	writeJSON(w, http.StatusOK, map[string]string{"message": "更新成功"})
}

func (h *Handler) deleteDepartment(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "部门编号无效")
		return
	}

	count, err := h.Queries.CountEmployeesByDepartment(r.Context(), toNullInt64(id))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "部门校验失败")
		return
	}
	if count > 0 {
		writeError(w, http.StatusBadRequest, "部门下仍有员工，无法删除")
		return
	}

	if err := h.Queries.DeleteDepartment(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "删除部门失败")
		return
	}

	h.logAudit(r, "delete_department", "department", sql.NullInt64{Int64: id, Valid: true}, nil)
	writeJSON(w, http.StatusOK, map[string]string{"message": "删除成功"})
}

func toNullInt64(value int64) sql.NullInt64 {
	if value <= 0 {
		return sql.NullInt64{Valid: false}
	}
	return sql.NullInt64{Int64: value, Valid: true}
}
