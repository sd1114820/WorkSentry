package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestReportJobStoreCoalescesSameReport(t *testing.T) {
	var store reportJobStore
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	load := func() reportJobLoadResult {
		calls.Add(1)
		close(started)
		<-release
		return reportJobLoadResult{value: "完成"}
	}

	first := store.getOrStart("daily:2026-08-01:0", load)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("报表生成任务未启动")
	}
	second := store.getOrStart("daily:2026-08-01:0", load)
	if first != second {
		t.Fatal("同一报表的并发请求应复用同一生成任务")
	}
	close(release)
	select {
	case <-first.done:
	case <-time.After(time.Second):
		t.Fatal("报表生成任务未完成")
	}
	if calls.Load() != 1 {
		t.Fatalf("报表查询执行次数 = %d，期望 1", calls.Load())
	}
	if first.result.value != "完成" {
		t.Fatalf("报表结果 = %v，期望完成", first.result.value)
	}
}

func TestWriteReportPendingReturnsPollContract(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeReportPending(recorder, "日统计汇总", time.Now().Add(-2*time.Second))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("状态码 = %d，期望 %d", recorder.Code, http.StatusAccepted)
	}

	var payload struct {
		Code string `json:"code"`
		Data struct {
			RetryAfterMS int64 `json:"retryAfterMs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if payload.Code != "report_pending" {
		t.Fatalf("响应代码 = %q，期望 report_pending", payload.Code)
	}
	if payload.Data.RetryAfterMS != reportJobPollDelay.Milliseconds() {
		t.Fatalf("轮询间隔 = %d，期望 %d", payload.Data.RetryAfterMS, reportJobPollDelay.Milliseconds())
	}
}

func TestWriteExportPendingIsNotDownloadableAsSuccessfulFile(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeExportPending(recorder, "日报导出文件", time.Now())
	if recorder.Code != http.StatusTooEarly {
		t.Fatalf("状态码 = %d，期望 %d", recorder.Code, http.StatusTooEarly)
	}

	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if payload.Code != "report_pending" {
		t.Fatalf("响应代码 = %q，期望 report_pending", payload.Code)
	}
}

func TestReportPendingResponseFitsFiveSecondGateway(t *testing.T) {
	const gatewayLimit = 5 * time.Second
	maximumHandlerWait := adminAuthenticationQueryTimeout + reportJobInitialWait
	if maximumHandlerWait >= gatewayLimit {
		t.Fatalf("会话校验与报表等待上限合计 = %s，必须小于网关的 %s", maximumHandlerWait, gatewayLimit)
	}
}
