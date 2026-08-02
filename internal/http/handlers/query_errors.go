package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

func writeDataQueryError(w http.ResponseWriter, resource string, queryParameters string, err error, startedAt time.Time) {
	errorID := newErrorReference()
	elapsed := time.Since(startedAt).Round(time.Millisecond)
	status := http.StatusInternalServerError
	code := "data_query_failed"
	message := fmt.Sprintf("读取%s失败：数据库查询异常（错误编号：%s）", resource, errorID)

	if errors.Is(err, context.DeadlineExceeded) {
		status = http.StatusGatewayTimeout
		code = "data_query_timeout"
		message = fmt.Sprintf("读取%s超时：数据库未在时限内返回（错误编号：%s）", resource, errorID)
	} else if errors.Is(err, context.Canceled) {
		code = "data_query_canceled"
		message = fmt.Sprintf("读取%s失败：请求已取消（错误编号：%s）", resource, errorID)
	}

	log.Printf("读取%s失败: 错误编号=%s 参数={%s} 耗时=%s 错误=%v", resource, errorID, queryParameters, elapsed, err)
	w.Header().Set("X-Error-ID", errorID)
	writeErrorWithCodeData(w, status, code, message, map[string]any{
		"errorId":   errorID,
		"elapsedMs": elapsed.Milliseconds(),
	})
}
