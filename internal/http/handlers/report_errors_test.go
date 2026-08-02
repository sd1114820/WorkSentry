package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"worksentry/internal/config"
)

func TestReportQueryContextIsIndependentFromHTTPWriteTimeout(t *testing.T) {
	h := &Handler{Config: &config.Config{Server: config.ServerConfig{
		WriteTimeoutSeconds:       9,
		ReportQueryTimeoutSeconds: 20,
	}}}
	ctx, cancel, timeout := h.reportQueryContext(context.Background())
	defer cancel()
	if timeout != 2*time.Minute {
		t.Fatalf("查询超时 = %s，期望 2m", timeout)
	}
	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("报表查询上下文应包含截止时间")
	}
}

func TestDefaultReportQueryTimeoutSupportsBackgroundGeneration(t *testing.T) {
	h := &Handler{}
	ctx, cancel, timeout := h.reportQueryContext(context.Background())
	defer cancel()
	if timeout != 2*time.Minute {
		t.Fatalf("默认报表查询超时 = %s，期望 2m", timeout)
	}
	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("默认报表查询上下文应包含截止时间")
	}
}

func TestOldFiveSecondReportTimeoutIsRaised(t *testing.T) {
	h := &Handler{Config: &config.Config{Server: config.ServerConfig{
		WriteTimeoutSeconds:       15,
		ReportQueryTimeoutSeconds: 5,
	}}}
	_, cancel, timeout := h.reportQueryContext(context.Background())
	defer cancel()
	if timeout != 2*time.Minute {
		t.Fatalf("旧配置的报表查询超时 = %s，期望提高到 2m", timeout)
	}
}

func TestLongerConfiguredReportTimeoutIsKept(t *testing.T) {
	h := &Handler{Config: &config.Config{Server: config.ServerConfig{
		ReportQueryTimeoutSeconds: 300,
	}}}
	_, cancel, timeout := h.reportQueryContext(context.Background())
	defer cancel()
	if timeout != 5*time.Minute {
		t.Fatalf("报表查询超时 = %s，期望 5m", timeout)
	}
}

func TestWriteReportQueryTimeoutReturnsDiagnosableError(t *testing.T) {
	h := &Handler{}
	recorder := httptest.NewRecorder()
	h.writeReportQueryError(recorder, "团队排行", "日期=2026-07-31", context.DeadlineExceeded, time.Now().Add(-10*time.Second), 10*time.Second)

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
	if payload.Code != "report_query_timeout" {
		t.Fatalf("错误代码 = %q", payload.Code)
	}
	if !strings.Contains(payload.Message, "查询超过 10 秒") || !strings.Contains(payload.Message, "错误编号") {
		t.Fatalf("错误信息不明确: %q", payload.Message)
	}
	if payload.Data["errorId"] == "" {
		t.Fatal("错误详情应包含错误编号")
	}
}
