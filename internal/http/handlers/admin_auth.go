package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"worksentry/internal/db/sqlc"
)

type adminContextKey struct{}

func (h *Handler) AdminOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, err := h.authenticateAdmin(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err.Error())
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
		return sqlc.AdminSession{}, errors.New("缺少管理员令牌")
	}

	session, err := h.Queries.GetAdminSession(r.Context(), token)
	if err == sql.ErrNoRows {
		return sqlc.AdminSession{}, errors.New("会话已失效")
	}
	if err != nil {
		return sqlc.AdminSession{}, errors.New("会话校验失败")
	}
	if session.Revoked {
		return sqlc.AdminSession{}, errors.New("会话已失效")
	}
	if session.ExpiresAt.Valid && session.ExpiresAt.Time.Before(time.Now()) {
		return sqlc.AdminSession{}, errors.New("会话已过期")
	}

	_ = h.Queries.UpdateAdminSessionLastSeen(r.Context(), sqlc.UpdateAdminSessionLastSeenParams{
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
