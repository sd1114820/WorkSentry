package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"worksentry/internal/db/sqlc"
)

const offlineSegmentResultLimit = 50000

var errOfflineSegmentsTooMany = errors.New("离线段数量超过单次展示上限")

type OfflineSegmentView struct {
	EmployeeCode string `json:"employeeCode"`
	Name         string `json:"name"`
	Department   string `json:"department"`
	StartAt      string `json:"startAt"`
	EndAt        string `json:"endAt"`
	Duration     string `json:"duration"`
}

type offlineSegmentMerger struct {
	views            []OfflineSegmentView
	lastEmployeeCode string
	mergedStart      time.Time
	mergedEnd        time.Time
}

func (h *Handler) OfflineSegments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
		return
	}
	dateValue := r.URL.Query().Get("date")
	code := strings.TrimSpace(r.URL.Query().Get("employeeCode"))
	var date time.Time
	var err error
	if dateValue == "" {
		date = time.Now()
	} else {
		date, err = parseDate(dateValue)
		if err != nil {
			writeError(w, http.StatusBadRequest, "日期格式错误")
			return
		}
	}

	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.Local)
	end := start.Add(24 * time.Hour)
	queryParameters := "date=" + start.Format("2006-01-02") + ", employeeCode=" + code
	jobKey := fmt.Sprintf("offline:%s:%s", start.Format("2006-01-02"), code)
	job := h.reportJobs.getOrStart(jobKey, func() reportJobLoadResult {
		return newReportJobLoadResult(h, func(queryContext context.Context) (any, error) {
			return h.loadOfflineSegmentViews(queryContext, start, end, code)
		})
	})
	result, completed := waitReportJob(r, job)
	if !completed {
		writeReportPending(w, "离线段列表", job.createdAt)
		return
	}
	if result.err != nil {
		if errors.Is(result.err, errOfflineSegmentsTooMany) {
			writeErrorWithCode(w, http.StatusUnprocessableEntity, "offline_segments_too_many", "当日离线段过多，请选择员工后再查询")
			return
		}
		h.writeReportQueryError(w, "离线段", queryParameters, result.err, result.startedAt, result.timeout)
		return
	}
	views, ok := result.value.([]OfflineSegmentView)
	if !ok {
		writeDataQueryError(w, "离线段", queryParameters, errors.New("查询结果类型错误"), result.startedAt)
		return
	}
	writeJSON(w, http.StatusOK, views)
}

func (h *Handler) loadOfflineSegmentViews(ctx context.Context, start time.Time, end time.Time, employeeCode string) ([]OfflineSegmentView, error) {
	if h.DB == nil {
		rows, err := h.Queries.ListOfflineSegmentsByDate(ctx, buildOfflineSegmentsByDateParams(start, end, employeeCode))
		if err != nil {
			return nil, err
		}
		views := make([]OfflineSegmentView, 0, len(rows))
		for _, row := range rows {
			views = append(views, offlineSegmentView(row.EmployeeCode, row.Name, row.DepartmentName, row.StartAt, row.EndAt))
		}
		return views, nil
	}

	rows, err := h.queryOfflineSegments(ctx, true, start, end, employeeCode)
	if isMissingForcedIndexError(err) {
		rows, err = h.queryOfflineSegments(ctx, false, start, end, employeeCode)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	merger := offlineSegmentMerger{views: make([]OfflineSegmentView, 0, 256)}
	for rows.Next() {
		var employeeID int64
		var code string
		var name string
		var department sql.NullString
		var rowStart time.Time
		var rowEnd time.Time
		if err := rows.Scan(&employeeID, &code, &name, &department, &rowStart, &rowEnd); err != nil {
			return nil, err
		}

		if !merger.add(code, name, department, rowStart, rowEnd) {
			return nil, errOfflineSegmentsTooMany
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return merger.views, nil
}

func (h *Handler) queryOfflineSegments(ctx context.Context, forceIndex bool, start time.Time, end time.Time, employeeCode string) (*sql.Rows, error) {
	indexHint := ""
	if forceIndex {
		indexHint = " FORCE INDEX (idx_time_segments_employee_end)"
	}
	query := `SELECT ts.employee_id,
       e.employee_code,
       e.name,
       d.name AS department_name,
       ts.start_at,
       ts.end_at
FROM employees e
STRAIGHT_JOIN time_segments ts` + indexHint + `
  ON ts.employee_id = e.id
 AND ts.status = 'offline'
 AND ts.start_at < ?
 AND ts.end_at > ?
LEFT JOIN departments d ON e.department_id = d.id
WHERE (? = '' OR e.employee_code = ?)
ORDER BY e.employee_code, ts.start_at`
	return h.DB.QueryContext(ctx, query, end, start, employeeCode, employeeCode)
}

func isMissingForcedIndexError(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1176
}

func offlineSegmentView(employeeCode string, name string, department sql.NullString, start time.Time, end time.Time) OfflineSegmentView {
	return OfflineSegmentView{
		EmployeeCode: employeeCode,
		Name:         name,
		Department:   nullString(department),
		StartAt:      formatTime(start),
		EndAt:        formatTime(end),
		Duration:     formatDuration(int64(end.Sub(start).Seconds())),
	}
}

func (m *offlineSegmentMerger) add(employeeCode string, name string, department sql.NullString, start time.Time, end time.Time) bool {
	if len(m.views) > 0 && employeeCode == m.lastEmployeeCode && !start.After(m.mergedEnd) {
		if end.After(m.mergedEnd) {
			m.mergedEnd = end
			last := &m.views[len(m.views)-1]
			last.EndAt = formatTime(m.mergedEnd)
			last.Duration = formatDuration(int64(m.mergedEnd.Sub(m.mergedStart).Seconds()))
		}
		return true
	}
	if len(m.views) >= offlineSegmentResultLimit {
		return false
	}
	m.views = append(m.views, offlineSegmentView(employeeCode, name, department, start, end))
	m.lastEmployeeCode = employeeCode
	m.mergedStart = start
	m.mergedEnd = end
	return true
}

func buildOfflineSegmentsByDateParams(start time.Time, end time.Time, employeeCode string) sqlc.ListOfflineSegmentsByDateParams {
	return sqlc.ListOfflineSegmentsByDateParams{
		RangeEnd:           end,
		RangeStart:         start,
		EmployeeCodeFilter: employeeCode,
		EmployeeCode:       employeeCode,
	}
}
