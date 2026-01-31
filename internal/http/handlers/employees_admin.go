package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"worksentry/internal/db/sqlc"
)

type EmployeePayload struct {
	ID           int64  `json:"id"`
	EmployeeCode string `json:"employeeCode"`
	Name         string `json:"name"`
	DepartmentID int64  `json:"departmentId"`
	Enabled      bool   `json:"enabled"`
}

type EmployeeView struct {
	ID           int64  `json:"id"`
	EmployeeCode string `json:"employeeCode"`
	Name         string `json:"name"`
	Department   string `json:"department"`
	DepartmentID int64  `json:"departmentId"`
	Enabled      bool   `json:"enabled"`
	BindStatus   string `json:"bindStatus"`
	LastSeen     string `json:"lastSeen"`
}

type UnbindPayload struct {
	ID int64 `json:"id"`
}

func (h *Handler) Employees(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listEmployees(w, r)
	case http.MethodPost:
		h.createEmployee(w, r)
	case http.MethodPut:
		h.updateEmployee(w, r)
	case http.MethodDelete:
		h.disableEmployee(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
	}
}

func (h *Handler) listEmployees(w http.ResponseWriter, r *http.Request) {
	keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))

	if keyword == "" {
		items, err := h.Queries.ListEmployeesAdmin(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "读取员工失败")
			return
		}

		views := make([]EmployeeView, 0, len(items))
		for _, item := range items {
			departmentID := int64(0)
			if item.DepartmentID.Valid {
				departmentID = item.DepartmentID.Int64
			}
			lastSeen := ""
			if item.LastSeenAt.Valid {
				lastSeen = formatTime(item.LastSeenAt.Time)
			}
			bindStatus := "未绑定"
			if item.FingerprintHash.Valid {
				bindStatus = "已绑定"
			}
			views = append(views, EmployeeView{
				ID:           item.ID,
				EmployeeCode: item.EmployeeCode,
				Name:         item.Name,
				Department:   nullString(item.DepartmentName),
				DepartmentID: departmentID,
				Enabled:      item.Enabled,
				BindStatus:   bindStatus,
				LastSeen:     lastSeen,
			})
		}

		writeJSON(w, http.StatusOK, views)
		return
	}

	like := "%" + keyword + "%"
	items, err := h.Queries.ListEmployeesAdminByKeyword(r.Context(), sqlc.ListEmployeesAdminByKeywordParams{
		EmployeeCode: like,
		Name:         like,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取员工失败")
		return
	}

	views := make([]EmployeeView, 0, len(items))
	for _, item := range items {
		departmentID := int64(0)
		if item.DepartmentID.Valid {
			departmentID = item.DepartmentID.Int64
		}
		lastSeen := ""
		if item.LastSeenAt.Valid {
			lastSeen = formatTime(item.LastSeenAt.Time)
		}
		bindStatus := "未绑定"
		if item.FingerprintHash.Valid {
			bindStatus = "已绑定"
		}
		views = append(views, EmployeeView{
			ID:           item.ID,
			EmployeeCode: item.EmployeeCode,
			Name:         item.Name,
			Department:   nullString(item.DepartmentName),
			DepartmentID: departmentID,
			Enabled:      item.Enabled,
			BindStatus:   bindStatus,
			LastSeen:     lastSeen,
		})
	}

	writeJSON(w, http.StatusOK, views)
}

func (h *Handler) createEmployee(w http.ResponseWriter, r *http.Request) {
	var payload EmployeePayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "参数格式错误")
		return
	}
	payload.EmployeeCode = strings.TrimSpace(payload.EmployeeCode)
	payload.Name = strings.TrimSpace(payload.Name)
	if payload.EmployeeCode == "" || payload.Name == "" {
		writeError(w, http.StatusBadRequest, "工号与姓名不能为空")
		return
	}

	if existing, err := h.Queries.GetEmployeeByCode(r.Context(), payload.EmployeeCode); err == nil {
		if existing.ID > 0 {
			writeError(w, http.StatusBadRequest, "工号已存在")
			return
		}
	} else if err != sql.ErrNoRows {
		writeError(w, http.StatusInternalServerError, "校验工号失败")
		return
	}

	if err := h.Queries.CreateEmployee(r.Context(), sqlc.CreateEmployeeParams{
		EmployeeCode: payload.EmployeeCode,
		Name:         payload.Name,
		DepartmentID: toNullInt64(payload.DepartmentID),
		Enabled:      payload.Enabled,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "新增员工失败")
		return
	}

	h.logAudit(r, "create_employee", "employee", sql.NullInt64{}, payload)
	writeJSON(w, http.StatusOK, map[string]string{"message": "保存成功"})
}

func (h *Handler) updateEmployee(w http.ResponseWriter, r *http.Request) {
	var payload EmployeePayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "参数格式错误")
		return
	}
	payload.EmployeeCode = strings.TrimSpace(payload.EmployeeCode)
	payload.Name = strings.TrimSpace(payload.Name)
	if payload.ID <= 0 {
		writeError(w, http.StatusBadRequest, "员工编号不能为空")
		return
	}
	if payload.EmployeeCode == "" || payload.Name == "" {
		writeError(w, http.StatusBadRequest, "工号与姓名不能为空")
		return
	}

	if existing, err := h.Queries.GetEmployeeByCode(r.Context(), payload.EmployeeCode); err == nil {
		if existing.ID != payload.ID {
			writeError(w, http.StatusBadRequest, "工号已存在")
			return
		}
	} else if err != sql.ErrNoRows {
		writeError(w, http.StatusInternalServerError, "校验工号失败")
		return
	}

	if err := h.Queries.UpdateEmployee(r.Context(), sqlc.UpdateEmployeeParams{
		ID:           payload.ID,
		EmployeeCode: payload.EmployeeCode,
		Name:         payload.Name,
		DepartmentID: toNullInt64(payload.DepartmentID),
		Enabled:      payload.Enabled,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "更新员工失败")
		return
	}

	if !payload.Enabled {
		_ = h.Queries.RevokeTokensByEmployee(r.Context(), payload.ID)
	}

	h.logAudit(r, "update_employee", "employee", sql.NullInt64{Int64: payload.ID, Valid: true}, payload)
	writeJSON(w, http.StatusOK, map[string]string{"message": "更新成功"})
}

func (h *Handler) disableEmployee(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "员工编号无效")
		return
	}

	if err := h.Queries.UpdateEmployeeEnabled(r.Context(), sqlc.UpdateEmployeeEnabledParams{Enabled: false, ID: id}); err != nil {
		writeError(w, http.StatusInternalServerError, "停用员工失败")
		return
	}
	_ = h.Queries.RevokeTokensByEmployee(r.Context(), id)

	h.logAudit(r, "disable_employee", "employee", sql.NullInt64{Int64: id, Valid: true}, nil)
	writeJSON(w, http.StatusOK, map[string]string{"message": "已停用"})
}

func (h *Handler) UnbindEmployee(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
		return
	}
	var payload UnbindPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "参数格式错误")
		return
	}
	if payload.ID <= 0 {
		writeError(w, http.StatusBadRequest, "员工编号不能为空")
		return
	}

	if err := h.Queries.ClearEmployeeFingerprint(r.Context(), payload.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "解绑失败")
		return
	}
	_ = h.Queries.RevokeTokensByEmployee(r.Context(), payload.ID)

	h.logAudit(r, "unbind_employee", "employee", sql.NullInt64{Int64: payload.ID, Valid: true}, nil)
	writeJSON(w, http.StatusOK, map[string]string{"message": "解绑成功"})
}
