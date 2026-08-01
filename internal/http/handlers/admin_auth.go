package handlers

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"worksentry/internal/db/sqlc"
)

type adminContextKey struct{}

const adminAuthenticationQueryTimeout = 3 * time.Second

type adminAuthenticationError struct {
	status  int
	code    string
	message string
	cause   error
}

func (e *adminAuthenticationError) Error() string {
	return e.message
}

func newAdminAuthenticationError(status int, code string, message string, cause error) error {
	return &adminAuthenticationError{status: status, code: code, message: message, cause: cause}
}

func (h *Handler) AdminOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, err := h.authenticateAdmin(r)
		if err != nil {
			var authErr *adminAuthenticationError
			if !errors.As(err, &authErr) {
				writeError(w, http.StatusUnauthorized, "会话已失效")
				return
			}
			if authErr.status >= http.StatusInternalServerError {
				errorID := newErrorReference()
				log.Printf("管理员会话校验失败: 错误编号=%s 路径=%s 错误=%v", errorID, r.URL.Path, authErr.cause)
				w.Header().Set("X-Error-ID", errorID)
				writeErrorWithCodeData(w, authErr.status, authErr.code, authErr.message+"（错误编号："+errorID+"）", map[string]string{"errorId": errorID})
				return
			}
			writeErrorWithCode(w, authErr.status, authErr.code, authErr.message)
			return
		}
		ctx := context.WithValue(r.Context(), adminContextKey{}, session.AdminID)
		next(w, r.WithContext(ctx))
	}
}

func (h *Handler) authenticateAdmin(r *http.Request) (sqlc.AdminSession, error) {
	token := readBearerToken(r)
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("token"))
	}
	if token == "" {
		return sqlc.AdminSession{}, newAdminAuthenticationError(http.StatusUnauthorized, "admin_token_missing", "缺少管理员令牌", nil)
	}

	authContext, cancel := context.WithTimeout(r.Context(), adminAuthenticationQueryTimeout)
	defer cancel()
	session, err := h.Queries.GetAdminSession(authContext, token)
	if err == sql.ErrNoRows {
		return sqlc.AdminSession{}, newAdminAuthenticationError(http.StatusUnauthorized, "admin_session_invalid", "会话已失效", nil)
	}
	if err != nil {
		return sqlc.AdminSession{}, newAdminAuthenticationError(http.StatusServiceUnavailable, "admin_auth_unavailable", "会话校验暂时不可用，请稍后重试", err)
	}
	if session.Revoked {
		return sqlc.AdminSession{}, newAdminAuthenticationError(http.StatusUnauthorized, "admin_session_invalid", "会话已失效", nil)
	}
	if session.ExpiresAt.Valid && session.ExpiresAt.Time.Before(time.Now()) {
		return sqlc.AdminSession{}, newAdminAuthenticationError(http.StatusUnauthorized, "admin_session_expired", "会话已过期", nil)
	}

	_ = h.Queries.UpdateAdminSessionLastSeen(authContext, sqlc.UpdateAdminSessionLastSeenParams{
		LastSeen: sql.NullTime{Time: time.Now(), Valid: true},
		Token:    token,
	})
	return session, nil
}

func adminIDFromContext(ctx context.Context) int64 {
	value := ctx.Value(adminContextKey{})
	id, ok := value.(int64)
	if !ok {
		return 0
	}
	return id
}

func adminIDFromRequest(r *http.Request) int64 {
	return adminIDFromContext(r.Context())
}
