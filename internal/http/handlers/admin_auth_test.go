package handlers

import (
	"errors"
	"net/http"
	"testing"
)

func TestDatabaseAuthenticationFailureDoesNotBecomeUnauthorized(t *testing.T) {
	databaseErr := errors.New("数据库繁忙")
	err := newAdminAuthenticationError(http.StatusServiceUnavailable, "admin_auth_unavailable", "会话校验暂时不可用", databaseErr)

	var authErr *adminAuthenticationError
	if !errors.As(err, &authErr) {
		t.Fatal("应保留管理员会话错误类型")
	}
	if authErr.status != http.StatusServiceUnavailable {
		t.Fatalf("数据库错误状态码 = %d，期望 %d", authErr.status, http.StatusServiceUnavailable)
	}
	if authErr.status == http.StatusUnauthorized {
		t.Fatal("数据库错误不得伪装成会话失效")
	}
}
