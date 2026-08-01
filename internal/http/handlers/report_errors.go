package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

const defaultReportQueryTimeout = 5 * time.Second

func (h *Handler) reportQueryContext(parent context.Context) (context.Context, context.CancelFunc, time.Duration) {
	timeout := defaultReportQueryTimeout
	if h.Config != nil && h.Config.Server.ReportQueryTimeoutSeconds > 0 {
		timeout = time.Duration(h.Config.Server.ReportQueryTimeoutSeconds) * time.Second
	}

	// 查询必须早于服务响应超时结束，否则无法把明确的错误返回给页面。
	if h.Config != nil && h.Config.Server.WriteTimeoutSeconds > 2 {
		writeLimit := time.Duration(h.Config.Server.WriteTimeoutSeconds-2) * time.Second
		if timeout > writeLimit {
			timeout = writeLimit
		}
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	return ctx, cancel, timeout
}

func (h *Handler) writeReportQueryError(w http.ResponseWriter, reportName string, queryParameters string, err error, startedAt time.Time, timeout time.Duration) {
	errorID := newErrorReference()
	elapsed := time.Since(startedAt).Round(time.Millisecond)
	status := http.StatusInternalServerError
	code := "report_query_failed"
	message := fmt.Sprintf("读取%s失败：数据库查询异常（错误编号：%s）", reportName, errorID)

	if errors.Is(err, context.DeadlineExceeded) {
		status = http.StatusGatewayTimeout
		code = "report_query_timeout"
		message = fmt.Sprintf("读取%s超时：查询超过 %d 秒（错误编号：%s）", reportName, int(timeout/time.Second), errorID)
	} else if errors.Is(err, context.Canceled) {
		code = "report_query_canceled"
		message = fmt.Sprintf("读取%s失败：请求已取消（错误编号：%s）", reportName, errorID)
	}

	log.Printf("读取%s失败: 错误编号=%s 参数={%s} 耗时=%s 错误=%v", reportName, errorID, queryParameters, elapsed, err)
	w.Header().Set("X-Error-ID", errorID)
	writeErrorWithCodeData(w, status, code, message, map[string]any{
		"errorId":   errorID,
		"elapsedMs": elapsed.Milliseconds(),
	})
}

func logReportQuerySuccess(reportName string, queryParameters string, rowCount int, startedAt time.Time) {
	log.Printf("读取%s成功: 参数={%s} 行数=%d 耗时=%s", reportName, queryParameters, rowCount, time.Since(startedAt).Round(time.Millisecond))
}
