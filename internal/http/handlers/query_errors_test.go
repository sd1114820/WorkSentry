package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWriteDataQueryErrorReturnsTraceableTimeout(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeDataQueryError(recorder, "离线段", "date=2026-08-02", context.DeadlineExceeded, time.Now().Add(-time.Second))

	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("状态码 = %d，期望 %d", recorder.Code, http.StatusGatewayTimeout)
	}
	if recorder.Header().Get("X-Error-ID") == "" {
		t.Fatal("响应应包含错误编号")
	}
	var payload struct {
		Message string         `json:"message"`
		Code    string         `json:"code"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("解析错误响应失败: %v", err)
	}
	if payload.Code != "data_query_timeout" || !strings.Contains(payload.Message, "读取离线段超时") {
		t.Fatalf("错误响应不明确: %#v", payload)
	}
}
