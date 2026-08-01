package handlers

import (
	"context"
	"database/sql"
	"errors"
	"log"
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

const adminLoginDatabaseTimeout = 5 * time.Second

type bootstrapAdminError struct {
	status  int
	message string
	cause   error
}

func (e *bootstrapAdminError) Error() string {
	return e.message
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

	loginContext, cancel := context.WithTimeout(r.Context(), adminLoginDatabaseTimeout)
	defer cancel()

	if err := h.ensureBootstrapAdmin(loginContext); err != nil {
		var bootstrapErr *bootstrapAdminError
		if !errors.As(err, &bootstrapErr) {
			writeError(w, http.StatusInternalServerError, "管理员初始化失败")
			return
		}
		if bootstrapErr.status >= http.StatusInternalServerError {
			errorID := newErrorReference()
			log.Printf("管理员初始化检查失败: 错误编号=%s 错误=%v", errorID, bootstrapErr.cause)
			w.Header().Set("X-Error-ID", errorID)
			writeErrorWithCodeData(w, bootstrapErr.status, "admin_bootstrap_failed", bootstrapErr.message+"（错误编号："+errorID+"）", map[string]string{"errorId": errorID})
			return
		}
		writeError(w, bootstrapErr.status, bootstrapErr.message)
		return
	}

	user, err := h.Queries.GetAdminUserByUsername(loginContext, payload.Username)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusUnauthorized, "账号或密码错误")
		return
	}
	if err != nil {
		writeAdminLoginDatabaseError(w, "读取管理员账号", err)
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
	err = h.Queries.CreateAdminSession(loginContext, sqlc.CreateAdminSessionParams{
		Token:     token,
		AdminID:   user.ID,
		IssuedAt:  now,
		ExpiresAt: sql.NullTime{Valid: false},
		LastSeen:  sql.NullTime{Time: now, Valid: true},
	})
	if err != nil {
		writeAdminLoginDatabaseError(w, "创建管理员会话", err)
		return
	}

	writeJSON(w, http.StatusOK, AdminLoginResponse{Token: token, DisplayName: user.DisplayName})
}

func writeAdminLoginDatabaseError(w http.ResponseWriter, stage string, err error) {
	errorID := newErrorReference()
	log.Printf("管理员登录数据库操作失败: 错误编号=%s 阶段=%s 错误=%v", errorID, stage, err)
	w.Header().Set("X-Error-ID", errorID)
	writeErrorWithCodeData(w, http.StatusServiceUnavailable, "admin_login_unavailable", "登录服务暂时不可用（错误编号："+errorID+"）", map[string]string{"errorId": errorID})
}

func (h *Handler) ensureBootstrapAdmin(ctx context.Context) error {
	count, err := h.Queries.CountAdminUsers(ctx)
	if err != nil {
		return &bootstrapAdminError{status: http.StatusServiceUnavailable, message: "管理员数据读取暂时不可用", cause: err}
	}
	if count > 0 {
		return nil
	}

	username := strings.TrimSpace(h.Config.App.Admin.Username)
	password := strings.TrimSpace(h.Config.App.Admin.Password)
	if username == "" || password == "" {
		return &bootstrapAdminError{status: http.StatusBadRequest, message: "请先在配置文件设置管理员账号"}
	}
	displayName := strings.TrimSpace(h.Config.App.Admin.DisplayName)
	if displayName == "" {
		displayName = username
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return &bootstrapAdminError{status: http.StatusInternalServerError, message: "管理员密码初始化失败", cause: err}
	}

	if err := h.Queries.CreateAdminUser(ctx, sqlc.CreateAdminUserParams{
		Username:     username,
		PasswordHash: string(hash),
		DisplayName:  displayName,
	}); err != nil {
		return &bootstrapAdminError{status: http.StatusInternalServerError, message: "管理员账号初始化失败", cause: err}
	}
	return nil
}
