package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

type AdminUserPayload struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
}

type AdminUserView struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	CreatedAt   string `json:"createdAt"`
	IsCurrent   bool   `json:"isCurrent"`
}

type AdminPasswordPayload struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (h *Handler) AdminUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listAdminUsers(w, r)
	case http.MethodPost:
		h.createAdminUser(w, r)
	case http.MethodPut:
		h.updateAdminUser(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
	}
}

func (h *Handler) AdminChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
		return
	}
	if h.DB == nil {
		writeError(w, http.StatusInternalServerError, "数据库未初始化")
		return
	}

	var payload AdminPasswordPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "参数格式错误")
		return
	}

	payload.CurrentPassword = strings.TrimSpace(payload.CurrentPassword)
	payload.NewPassword = strings.TrimSpace(payload.NewPassword)
	if payload.CurrentPassword == "" || payload.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "原密码和新密码不能为空")
		return
	}
	if len(payload.NewPassword) < 6 {
		writeError(w, http.StatusBadRequest, "新密码至少 6 位")
		return
	}
	if payload.CurrentPassword == payload.NewPassword {
		writeError(w, http.StatusBadRequest, "新密码不能与原密码相同")
		return
	}

	adminID := adminIDFromRequest(r)
	if adminID <= 0 {
		writeError(w, http.StatusUnauthorized, "登录状态已失效")
		return
	}

	var currentHash string
	err := h.DB.QueryRowContext(r.Context(), `SELECT password_hash FROM admin_users WHERE id = ? LIMIT 1`, adminID).Scan(&currentHash)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusUnauthorized, "登录状态已失效")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取账号失败")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(payload.CurrentPassword)); err != nil {
		writeError(w, http.StatusBadRequest, "原密码错误")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(payload.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "密码加密失败")
		return
	}

	if _, err := h.DB.ExecContext(r.Context(), `UPDATE admin_users SET password_hash = ? WHERE id = ?`, string(hash), adminID); err != nil {
		writeError(w, http.StatusInternalServerError, "修改密码失败")
		return
	}

	h.logAudit(r, "change_admin_password", "admin_users", sql.NullInt64{Int64: adminID, Valid: true}, nil)
	writeJSON(w, http.StatusOK, map[string]string{"message": "修改成功"})
}

func (h *Handler) listAdminUsers(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		writeError(w, http.StatusInternalServerError, "数据库未初始化")
		return
	}

	rows, err := h.DB.QueryContext(r.Context(), `
SELECT id, username, display_name, created_at
FROM admin_users
ORDER BY id ASC
`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取管理员失败")
		return
	}
	defer rows.Close()

	currentID := adminIDFromRequest(r)
	views := make([]AdminUserView, 0)
	for rows.Next() {
		var item AdminUserView
		var createdAt sql.NullTime
		if err := rows.Scan(&item.ID, &item.Username, &item.DisplayName, &createdAt); err != nil {
			writeError(w, http.StatusInternalServerError, "读取管理员失败")
			return
		}
		if createdAt.Valid {
			item.CreatedAt = formatTime(createdAt.Time)
		}
		item.IsCurrent = item.ID == currentID
		views = append(views, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "读取管理员失败")
		return
	}

	writeJSON(w, http.StatusOK, views)
}

func (h *Handler) createAdminUser(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		writeError(w, http.StatusInternalServerError, "数据库未初始化")
		return
	}

	var payload AdminUserPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "参数格式错误")
		return
	}

	payload.Username = strings.TrimSpace(payload.Username)
	payload.DisplayName = strings.TrimSpace(payload.DisplayName)
	payload.Password = strings.TrimSpace(payload.Password)

	if payload.Username == "" {
		writeError(w, http.StatusBadRequest, "管理员账号不能为空")
		return
	}
	if payload.DisplayName == "" {
		payload.DisplayName = payload.Username
	}
	if payload.Password == "" {
		writeError(w, http.StatusBadRequest, "管理员密码不能为空")
		return
	}
	if len(payload.Password) < 6 {
		writeError(w, http.StatusBadRequest, "管理员密码至少 6 位")
		return
	}

	exists, err := h.adminUsernameExists(r.Context(), payload.Username, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "校验管理员账号失败")
		return
	}
	if exists {
		writeError(w, http.StatusBadRequest, "管理员账号已存在")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(payload.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "密码加密失败")
		return
	}

	result, err := h.DB.ExecContext(r.Context(), `INSERT INTO admin_users (username, password_hash, display_name) VALUES (?, ?, ?)`, payload.Username, string(hash), payload.DisplayName)
	if err != nil {
		if isAdminDuplicateErr(err) {
			writeError(w, http.StatusBadRequest, "管理员账号已存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "新增管理员失败")
		return
	}

	insertedID, _ := result.LastInsertId()
	h.logAudit(r, "create_admin_user", "admin_users", sql.NullInt64{Int64: insertedID, Valid: insertedID > 0}, map[string]any{
		"username":    payload.Username,
		"displayName": payload.DisplayName,
	})
	writeJSON(w, http.StatusOK, map[string]string{"message": "保存成功"})
}

func (h *Handler) updateAdminUser(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		writeError(w, http.StatusInternalServerError, "数据库未初始化")
		return
	}

	var payload AdminUserPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "参数格式错误")
		return
	}

	payload.Username = strings.TrimSpace(payload.Username)
	payload.DisplayName = strings.TrimSpace(payload.DisplayName)
	payload.Password = strings.TrimSpace(payload.Password)

	if payload.ID <= 0 {
		writeError(w, http.StatusBadRequest, "管理员ID不能为空")
		return
	}
	if payload.Username == "" {
		writeError(w, http.StatusBadRequest, "管理员账号不能为空")
		return
	}
	if payload.DisplayName == "" {
		payload.DisplayName = payload.Username
	}
	if payload.Password != "" && len(payload.Password) < 6 {
		writeError(w, http.StatusBadRequest, "重置密码至少 6 位")
		return
	}

	var existsID int64
	if err := h.DB.QueryRowContext(r.Context(), `SELECT id FROM admin_users WHERE id = ? LIMIT 1`, payload.ID).Scan(&existsID); err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "管理员不存在")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "读取管理员失败")
		return
	}

	exists, err := h.adminUsernameExists(r.Context(), payload.Username, payload.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "校验管理员账号失败")
		return
	}
	if exists {
		writeError(w, http.StatusBadRequest, "管理员账号已存在")
		return
	}

	if payload.Password == "" {
		if _, err := h.DB.ExecContext(r.Context(), `UPDATE admin_users SET username = ?, display_name = ? WHERE id = ?`, payload.Username, payload.DisplayName, payload.ID); err != nil {
			if isAdminDuplicateErr(err) {
				writeError(w, http.StatusBadRequest, "管理员账号已存在")
				return
			}
			writeError(w, http.StatusInternalServerError, "更新管理员失败")
			return
		}
	} else {
		hash, err := bcrypt.GenerateFromPassword([]byte(payload.Password), bcrypt.DefaultCost)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "密码加密失败")
			return
		}
		if _, err := h.DB.ExecContext(r.Context(), `UPDATE admin_users SET username = ?, display_name = ?, password_hash = ? WHERE id = ?`, payload.Username, payload.DisplayName, string(hash), payload.ID); err != nil {
			if isAdminDuplicateErr(err) {
				writeError(w, http.StatusBadRequest, "管理员账号已存在")
				return
			}
			writeError(w, http.StatusInternalServerError, "更新管理员失败")
			return
		}
	}

	h.logAudit(r, "update_admin_user", "admin_users", sql.NullInt64{Int64: payload.ID, Valid: true}, map[string]any{
		"username":    payload.Username,
		"displayName": payload.DisplayName,
		"resetPwd":    payload.Password != "",
	})
	writeJSON(w, http.StatusOK, map[string]string{"message": "保存成功"})
}

func (h *Handler) adminUsernameExists(ctx context.Context, username string, excludeID int64) (bool, error) {
	if h.DB == nil {
		return false, errors.New("db not ready")
	}

	var id int64
	err := h.DB.QueryRowContext(ctx, `SELECT id FROM admin_users WHERE username = ? LIMIT 1`, username).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if excludeID > 0 && id == excludeID {
		return false, nil
	}
	return true, nil
}

func isAdminDuplicateErr(err error) bool {
	if err == nil {
		return false
	}
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	return strings.Contains(strings.ToLower(err.Error()), "duplicate")
}
