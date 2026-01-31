package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"worksentry/internal/db/sqlc"
)

type AdminLoginPayload struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AdminLoginResponse struct {
	Token       string `json:"token"`
	DisplayName string `json:"displayName"`
}

func (h *Handler) AdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
		return
	}

	var payload AdminLoginPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "参数格式错误")
		return
	}
	payload.Username = strings.TrimSpace(payload.Username)
	payload.Password = strings.TrimSpace(payload.Password)

	if payload.Username == "" || payload.Password == "" {
		writeError(w, http.StatusBadRequest, "账号与密码不能为空")
		return
	}

	if err := h.ensureBootstrapAdmin(r.Context()); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	user, err := h.Queries.GetAdminUserByUsername(r.Context(), payload.Username)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusUnauthorized, "账号或密码错误")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "登录失败")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(payload.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "账号或密码错误")
		return
	}

	token, err := generateToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "生成会话失败")
		return
	}

	now := time.Now()
	err = h.Queries.CreateAdminSession(r.Context(), sqlc.CreateAdminSessionParams{
		Token:     token,
		AdminID:   user.ID,
		IssuedAt:  now,
		ExpiresAt: sql.NullTime{Valid: false},
		LastSeen:  sql.NullTime{Time: now, Valid: true},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "创建会话失败")
		return
	}

	writeJSON(w, http.StatusOK, AdminLoginResponse{Token: token, DisplayName: user.DisplayName})
}

func (h *Handler) ensureBootstrapAdmin(ctx context.Context) error {
	count, err := h.Queries.CountAdminUsers(ctx)
	if err != nil {
		return fmt.Errorf("管理员初始化失败")
	}
	if count > 0 {
		return nil
	}

	username := strings.TrimSpace(h.Config.App.Admin.Username)
	password := strings.TrimSpace(h.Config.App.Admin.Password)
	if username == "" || password == "" {
		return fmt.Errorf("请先在配置文件设置管理员账号")
	}
	displayName := strings.TrimSpace(h.Config.App.Admin.DisplayName)
	if displayName == "" {
		displayName = username
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("管理员初始化失败")
	}

	if err := h.Queries.CreateAdminUser(ctx, sqlc.CreateAdminUserParams{
		Username:     username,
		PasswordHash: string(hash),
		DisplayName:  displayName,
	}); err != nil {
		return fmt.Errorf("管理员初始化失败")
	}
	return nil
}
