package handlers

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const (
	reportJobResultTTL      = 15 * time.Second
	reportJobInitialWait    = 1 * time.Second
	reportJobPollDelay      = 300 * time.Millisecond
	reportJobMaxConcurrency = 2
)

type reportJobLoadResult struct {
	value     any
	err       error
	startedAt time.Time
	timeout   time.Duration
}

type reportJob struct {
	done        chan struct{}
	createdAt   time.Time
	result      reportJobLoadResult
	completedAt time.Time
}

type reportJobStore struct {
	mu    sync.Mutex
	jobs  map[string]*reportJob
	slots chan struct{}
}

func (s *reportJobStore) getOrStart(key string, load func() reportJobLoadResult) *reportJob {
	now := time.Now()
	s.mu.Lock()
	if s.jobs == nil {
		s.jobs = make(map[string]*reportJob)
		s.slots = make(chan struct{}, reportJobMaxConcurrency)
	}
	for existingKey, existingJob := range s.jobs {
		if !existingJob.completedAt.IsZero() && now.Sub(existingJob.completedAt) > reportJobResultTTL {
			delete(s.jobs, existingKey)
		}
	}
	if existingJob, ok := s.jobs[key]; ok {
		s.mu.Unlock()
		return existingJob
	}

	job := &reportJob{done: make(chan struct{}), createdAt: now}
	s.jobs[key] = job
	slots := s.slots
	s.mu.Unlock()

	go func() {
		slots <- struct{}{}
		result := load()
		<-slots

		s.mu.Lock()
		job.result = result
		job.completedAt = time.Now()
		close(job.done)
		s.mu.Unlock()
	}()

	return job
}

func waitReportJob(r *http.Request, job *reportJob) (reportJobLoadResult, bool) {
	timer := time.NewTimer(reportJobInitialWait)
	defer timer.Stop()

	select {
	case <-job.done:
		return job.result, true
	case <-timer.C:
		return reportJobLoadResult{}, false
	case <-r.Context().Done():
		return reportJobLoadResult{}, false
	}
}

func writeReportPending(w http.ResponseWriter, reportName string, startedAt time.Time) {
	writePendingStatus(w, http.StatusAccepted, reportName, startedAt)
}

func writeExportPending(w http.ResponseWriter, reportName string, startedAt time.Time) {
	// 旧版页面会把所有 2xx 响应都当成导出文件下载。
	// 导出未完成时使用非 2xx，可避免旧页面把 JSON 保存成损坏的 xlsx。
	writePendingStatus(w, http.StatusTooEarly, reportName, startedAt)
}

func writePendingStatus(w http.ResponseWriter, status int, reportName string, startedAt time.Time) {
	elapsedSeconds := int64(time.Since(startedAt).Seconds())
	if elapsedSeconds < 0 {
		elapsedSeconds = 0
	}
	w.Header().Set("Retry-After", "1")
	writeJSON(w, status, map[string]any{
		"code":    "report_pending",
		"message": fmt.Sprintf("正在生成%s，已执行 %d 秒", reportName, elapsedSeconds),
		"data": map[string]any{
			"retryAfterMs": reportJobPollDelay.Milliseconds(),
		},
	})
}

func newReportJobLoadResult(h *Handler, load func(context.Context) (any, error)) reportJobLoadResult {
	startedAt := time.Now()
	queryContext, cancel, timeout := h.reportQueryContext(context.Background())
	defer cancel()
	value, err := load(queryContext)
	return reportJobLoadResult{
		value:     value,
		err:       err,
		startedAt: startedAt,
		timeout:   timeout,
	}
}
