package handlers

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "net/http"
    "strconv"
    "strings"
    "time"

    "worksentry/internal/db/sqlc"
)

type WorkSessionViolation struct {
    Type          string `json:"type"`
    StatusCode    string `json:"statusCode,omitempty"`
    StatusLabel   string `json:"statusLabel,omitempty"`
    TriggerAction string `json:"triggerAction"`
    ActualSeconds int64  `json:"actualSeconds"`
    LimitSeconds  int64  `json:"limitSeconds"`
    LimitType     string `json:"limitType"`
    Message       string `json:"message"`
}

type WorkSessionReviewPayload struct {
    WorkStandardSeconds int64                        `json:"workStandardSeconds"`
    BreakSeconds        int64                        `json:"breakSeconds"`
    StatusTotals        map[string]int64             `json:"statusTotals"`
    Violations          []WorkSessionViolation       `json:"violations"`
}

type WorkSessionReviewListItem struct {
    ID              int64  `json:"id"`
    WorkDate        string `json:"workDate"`
    EmployeeCode    string `json:"employeeCode"`
    Name            string `json:"name"`
    Department      string `json:"department"`
    StartAt         string `json:"startAt"`
    EndAt           string `json:"endAt"`
    WorkStandard    string `json:"workStandard"`
    BreakDuration   string `json:"breakDuration"`
    ViolationSummary string `json:"violationSummary"`
    ReasonStatus    string `json:"reasonStatus"`
}

type WorkSessionReviewListResponse struct {
    Total int64                     `json:"total"`
    Items []WorkSessionReviewListItem `json:"items"`
}

type WorkSessionReviewDetail struct {
    ID              int64                  `json:"id"`
    WorkDate        string                 `json:"workDate"`
    EmployeeCode    string                 `json:"employeeCode"`
    Name            string                 `json:"name"`
    Department      string                 `json:"department"`
    StartAt         string                 `json:"startAt"`
    EndAt           string                 `json:"endAt"`
    WorkStandard    string                 `json:"workStandard"`
    BreakDuration   string                 `json:"breakDuration"`
    ReasonStatus    string                 `json:"reasonStatus"`
    Reason          string                 `json:"reason"`
    Violations      []WorkSessionViolation `json:"violations"`
    StatusTotals    map[string]string      `json:"statusTotals"`
}

type departmentRule struct {
    TargetSeconds         int64
    MaxBreakSeconds       int64
    MaxBreakCount         int64
    MaxBreakSingleSeconds sql.NullInt64
    Exists                bool
}

type statusThreshold struct {
    StatusCode    string
    MinSeconds    sql.NullInt64
    MaxSeconds    sql.NullInt64
    TriggerAction string
    Enabled       bool
}

func (h *Handler) handleWorkEndAfterReport(ctx context.Context, employee sqlc.Employee, payload ClientReportRequest, now time.Time) (bool, error) {
    if h.DB == nil {
        return false, fmt.Errorf("数据库未初始化")
    }

    session, err := h.Queries.GetOpenWorkSessionByEmployee(ctx, employee.ID)
    if err == sql.ErrNoRows {
        return false, fmt.Errorf("未找到上班记录")
    }
    if err != nil {
        return false, fmt.Errorf("读取上班记录失败")
    }

    var template *sqlc.CheckoutTemplate
    var fields []sqlc.CheckoutField
    if employee.DepartmentID.Valid && employee.DepartmentID.Int64 > 0 {
        tpl, err := h.Queries.GetEnabledCheckoutTemplateByDepartment(ctx, employee.DepartmentID.Int64)
        if err != nil && err != sql.ErrNoRows {
            return false, fmt.Errorf("读取下班模板失败")
        }
        if err == nil {
            template = &tpl
            fields, err = h.Queries.ListCheckoutFieldsByTemplate(ctx, tpl.ID)
            if err != nil {
                return false, fmt.Errorf("读取下班字段失败")
            }
        }
    }

    var cleanedCheckout map[string]string
    if template != nil {
        if payload.Checkout == nil {
            return false, fmt.Errorf("请填写下班信息")
        }
        if payload.Checkout.TemplateID != template.ID {
            return false, fmt.Errorf("下班模板已更新，请刷新后重试")
        }
        snapshot := buildCheckoutSnapshot(*template, fields)
        cleanedCheckout, err = validateCheckoutData(snapshot.Fields, payload.Checkout.Data)
        if err != nil {
            return false, err
        }
    }

    violations := []WorkSessionViolation{}
    needReason := false

    configured := false
    var rule departmentRule
    var thresholds []statusThreshold

    if employee.DepartmentID.Valid && employee.DepartmentID.Int64 > 0 {
        rule, thresholds, configured, err = h.loadDepartmentRules(ctx, employee.DepartmentID.Int64)
        if err != nil {
            return false, fmt.Errorf("读取考核规则失败")
        }
    }

    workStandardSeconds := int64(now.Sub(session.StartAt).Seconds())
    breakSummary := breakSummaryResult{TotalSeconds: 0, Count: 0, MaxSingleSeconds: 0}
    statusTotals := defaultStatusTotals()

    if configured {
        breakSummary, err = h.calcBreakSummary(ctx, employee.ID, session.StartAt, now)
        if err != nil {
            return false, fmt.Errorf("计算休息时长失败")
        }
        statusTotals, err = h.calcStatusTotals(ctx, employee.ID, session.StartAt, now)
        if err != nil {
            return false, fmt.Errorf("计算状态时长失败")
        }
        workStandardSeconds = workStandardSeconds - breakSummary.TotalSeconds
        if workStandardSeconds < 0 {
            workStandardSeconds = 0
        }
    }

    if configured && rule.TargetSeconds > 0 && workStandardSeconds < rule.TargetSeconds {
        return false, &workEndError{
            Status:  http.StatusBadRequest,
            Code:    "work_time_short",
            Message: "工时标准未达标，无法下班",
        }
    }

    if configured {
        if rule.MaxBreakSeconds > 0 && breakSummary.TotalSeconds > rule.MaxBreakSeconds {
            violations = append(violations, WorkSessionViolation{
                Type:          "break_total",
                TriggerAction: triggerRequireReason,
                ActualSeconds: breakSummary.TotalSeconds,
                LimitSeconds:  rule.MaxBreakSeconds,
                LimitType:     "max",
                Message:       fmt.Sprintf("休息总时长超过限制（%s）", formatDuration(rule.MaxBreakSeconds)),
            })
        }
        if rule.MaxBreakCount > 0 && int64(breakSummary.Count) > rule.MaxBreakCount {
            violations = append(violations, WorkSessionViolation{
                Type:          "break_count",
                TriggerAction: triggerRequireReason,
                ActualSeconds: int64(breakSummary.Count),
                LimitSeconds:  rule.MaxBreakCount,
                LimitType:     "max",
                Message:       fmt.Sprintf("休息次数超过限制（%d 次）", rule.MaxBreakCount),
            })
        }
        if rule.MaxBreakSingleSeconds.Valid && rule.MaxBreakSingleSeconds.Int64 > 0 && breakSummary.MaxSingleSeconds > rule.MaxBreakSingleSeconds.Int64 {
            violations = append(violations, WorkSessionViolation{
                Type:          "break_single",
                TriggerAction: triggerRequireReason,
                ActualSeconds: breakSummary.MaxSingleSeconds,
                LimitSeconds:  rule.MaxBreakSingleSeconds.Int64,
                LimitType:     "max",
                Message:       fmt.Sprintf("单次休息超过限制（%s）", formatDuration(rule.MaxBreakSingleSeconds.Int64)),
            })
        }

        for _, item := range thresholds {
            if !item.Enabled {
                continue
            }
            actual := statusTotals[item.StatusCode]
            label := statusLabel(item.StatusCode)
            if item.MinSeconds.Valid && item.MinSeconds.Int64 > 0 && actual < item.MinSeconds.Int64 {
                violations = append(violations, WorkSessionViolation{
                    Type:          "status_threshold",
                    StatusCode:    item.StatusCode,
                    StatusLabel:   label,
                    TriggerAction: item.TriggerAction,
                    ActualSeconds: actual,
                    LimitSeconds:  item.MinSeconds.Int64,
                    LimitType:     "min",
                    Message:       fmt.Sprintf("%s时长未达标（少于 %s）", label, formatDuration(item.MinSeconds.Int64)),
                })
            }
            if item.MaxSeconds.Valid && item.MaxSeconds.Int64 > 0 && actual > item.MaxSeconds.Int64 {
                violations = append(violations, WorkSessionViolation{
                    Type:          "status_threshold",
                    StatusCode:    item.StatusCode,
                    StatusLabel:   label,
                    TriggerAction: item.TriggerAction,
                    ActualSeconds: actual,
                    LimitSeconds:  item.MaxSeconds.Int64,
                    LimitType:     "max",
                    Message:       fmt.Sprintf("%s时长超限（超过 %s）", label, formatDuration(item.MaxSeconds.Int64)),
                })
            }
        }
    }

    for _, v := range violations {
        if v.TriggerAction == triggerRequireReason {
            needReason = true
            break
        }
    }

    reason := strings.TrimSpace(payload.Reason)
    if needReason && reason == "" {
        return false, &workEndError{
            Status:  http.StatusConflict,
            Code:    "need_reason",
            Message: "下班需要补录原因",
            Data:    buildReviewPayload(workStandardSeconds, breakSummary.TotalSeconds, statusTotals, violations),
        }
    }

    if h.DB == nil {
        return false, fmt.Errorf("数据库未初始化")
    }

    tx, err := h.DB.BeginTx(ctx, nil)
    if err != nil {
        return false, fmt.Errorf("提交下班失败")
    }
    qtx := h.Queries.WithTx(tx)

    if template != nil {
        if _, err := qtx.GetWorkSessionCheckoutBySessionID(ctx, session.ID); err == nil {
            _ = tx.Rollback()
            return false, fmt.Errorf("下班录入已提交")
        } else if err != sql.ErrNoRows {
            _ = tx.Rollback()
            return false, fmt.Errorf("读取下班记录失败")
        }

        snapshot := buildCheckoutSnapshot(*template, fields)
        snapshotJSON, err := json.Marshal(snapshot)
        if err != nil {
            _ = tx.Rollback()
            return false, fmt.Errorf("生成下班快照失败")
        }
        dataJSON, err := json.Marshal(cleanedCheckout)
        if err != nil {
            _ = tx.Rollback()
            return false, fmt.Errorf("生成下班数据失败")
        }
        if err := qtx.CreateWorkSessionCheckout(ctx, sqlc.CreateWorkSessionCheckoutParams{
            WorkSessionID:        session.ID,
            TemplateID:           template.ID,
            TemplateSnapshotJSON: string(snapshotJSON),
            DataJSON:             string(dataJSON),
        }); err != nil {
            _ = tx.Rollback()
            return false, fmt.Errorf("保存下班录入失败")
        }
    }

    if len(violations) > 0 {
        payloadJSON, err := json.Marshal(buildReviewPayload(workStandardSeconds, breakSummary.TotalSeconds, statusTotals, violations))
        if err != nil {
            _ = tx.Rollback()
            return false, fmt.Errorf("生成考核记录失败")
        }
        _, err = tx.ExecContext(ctx, `INSERT INTO work_session_reviews (work_session_id, employee_id, department_id, work_date, work_standard_seconds, break_seconds, need_reason, reason, violations_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
            session.ID,
            employee.ID,
            nullIfZeroInt64(employee.DepartmentID),
            workDate(session.StartAt),
            workStandardSeconds,
            breakSummary.TotalSeconds,
            boolToTinyInt(needReason),
            toNullText(reason),
            string(payloadJSON),
        )
        if err != nil {
            _ = tx.Rollback()
            return false, fmt.Errorf("保存考核记录失败")
        }
    }

    if err := qtx.CloseWorkSession(ctx, sqlc.CloseWorkSessionParams{
        EmployeeID: employee.ID,
        EndAt:      now,
    }); err != nil {
        _ = tx.Rollback()
        return false, fmt.Errorf("写入下班记录失败")
    }

    if err := tx.Commit(); err != nil {
        return false, fmt.Errorf("提交下班失败")
    }

    return true, nil
}

type workEndError struct {
    Status  int
    Code    string
    Message string
    Data    any
}

func (e *workEndError) Error() string {
    return e.Message
}

func buildReviewPayload(workStandardSeconds int64, breakSeconds int64, totals map[string]int64, violations []WorkSessionViolation) WorkSessionReviewPayload {
    return WorkSessionReviewPayload{
        WorkStandardSeconds: workStandardSeconds,
        BreakSeconds:        breakSeconds,
        StatusTotals:        totals,
        Violations:          violations,
    }
}

func (h *Handler) loadDepartmentRules(ctx context.Context, departmentID int64) (departmentRule, []statusThreshold, bool, error) {
    rule := departmentRule{}
    row := h.DB.QueryRowContext(ctx, `SELECT target_seconds, max_break_seconds, max_break_count, max_break_single_seconds
FROM department_work_rules WHERE department_id = ?`, departmentID)
    if err := row.Scan(&rule.TargetSeconds, &rule.MaxBreakSeconds, &rule.MaxBreakCount, &rule.MaxBreakSingleSeconds); err == nil {
        rule.Exists = true
    } else if err != sql.ErrNoRows {
        return rule, nil, false, err
    }

    rows, err := h.DB.QueryContext(ctx, `SELECT status_code, min_seconds, max_seconds, trigger_action, enabled
FROM department_status_thresholds WHERE department_id = ?`, departmentID)
    if err != nil {
        return rule, nil, rule.Exists, err
    }
    defer rows.Close()

    thresholds := []statusThreshold{}
    for rows.Next() {
        var item statusThreshold
        var enabled int
        if err := rows.Scan(&item.StatusCode, &item.MinSeconds, &item.MaxSeconds, &item.TriggerAction, &enabled); err != nil {
            return rule, nil, rule.Exists, err
        }
        item.Enabled = enabled > 0
        thresholds = append(thresholds, item)
    }
    configured := rule.Exists || len(thresholds) > 0
    return rule, thresholds, configured, rows.Err()
}

type breakSummaryResult struct {
    TotalSeconds     int64
    Count            int32
    MaxSingleSeconds int64
}

func (h *Handler) calcBreakSummary(ctx context.Context, employeeID int64, start time.Time, end time.Time) (breakSummaryResult, error) {
    row := h.DB.QueryRowContext(ctx, `SELECT
COUNT(1) AS break_count,
IFNULL(SUM(TIMESTAMPDIFF(SECOND, GREATEST(start_at, ?), LEAST(end_at, ?))), 0) AS break_seconds,
IFNULL(MAX(TIMESTAMPDIFF(SECOND, GREATEST(start_at, ?), LEAST(end_at, ?))), 0) AS max_single
FROM time_segments
WHERE employee_id = ?
  AND status = 'break'
  AND start_at < ?
  AND end_at > ?`, start, end, start, end, employeeID, end, start)

    result := breakSummaryResult{}
    if err := row.Scan(&result.Count, &result.TotalSeconds, &result.MaxSingleSeconds); err != nil {
        return result, err
    }
    return result, nil
}

func (h *Handler) calcStatusTotals(ctx context.Context, employeeID int64, start time.Time, end time.Time) (map[string]int64, error) {
    totals := defaultStatusTotals()
    rows, err := h.DB.QueryContext(ctx, `SELECT status, IFNULL(SUM(TIMESTAMPDIFF(SECOND, GREATEST(start_at, ?), LEAST(end_at, ?))), 0) AS seconds
FROM time_segments
WHERE employee_id = ?
  AND start_at < ?
  AND end_at > ?
GROUP BY status`, start, end, employeeID, end, start)
    if err != nil {
        return totals, err
    }
    defer rows.Close()

    for rows.Next() {
        var status string
        var seconds int64
        if err := rows.Scan(&status, &seconds); err != nil {
            return totals, err
        }
        totals[status] = seconds
    }
    return totals, rows.Err()
}

func defaultStatusTotals() map[string]int64 {
    return map[string]int64{
        "work":    0,
        "normal":  0,
        "fish":    0,
        "idle":    0,
        "offline": 0,
        "break":   0,
    }
}

func workDate(start time.Time) string {
    return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location()).Format("2006-01-02")
}

func nullIfZeroInt64(value sql.NullInt64) interface{} {
    if value.Valid && value.Int64 > 0 {
        return value.Int64
    }
    return nil
}

func toNullText(value string) interface{} {
    if strings.TrimSpace(value) == "" {
        return nil
    }
    return value
}

func (h *Handler) WorkSessionReviews(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
        return
    }
    if h.DB == nil {
        writeError(w, http.StatusInternalServerError, "数据库未初始化")
        return
    }

    startValue := r.URL.Query().Get("startDate")
    endValue := r.URL.Query().Get("endDate")
    departmentID := parseInt64(r.URL.Query().Get("departmentId"))
    keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))

    if startValue == "" {
        startValue = time.Now().Format("2006-01-02")
    }
    if endValue == "" {
        endValue = startValue
    }

    startDate, err := parseDate(startValue)
    if err != nil {
        writeError(w, http.StatusBadRequest, "开始日期格式错误")
        return
    }
    endDate, err := parseDate(endValue)
    if err != nil {
        writeError(w, http.StatusBadRequest, "结束日期格式错误")
        return
    }

    page := parseInt(r.URL.Query().Get("page"), 1)
    pageSize := parseInt(r.URL.Query().Get("pageSize"), 20)
    if pageSize <= 0 || pageSize > 200 {
        pageSize = 20
    }
    if page <= 0 {
        page = 1
    }

    where := "WHERE r.work_date >= ? AND r.work_date <= ?"
    args := []any{startDate.Format("2006-01-02"), endDate.Format("2006-01-02")}

    if departmentID > 0 {
        where += " AND e.department_id = ?"
        args = append(args, departmentID)
    }
    if keyword != "" {
        where += " AND (e.employee_code LIKE ? OR e.name LIKE ?)"
        like := "%" + keyword + "%"
        args = append(args, like, like)
    }

    countSQL := "SELECT COUNT(1) FROM work_session_reviews r JOIN employees e ON r.employee_id = e.id " + where
    var total int64
    if err := h.DB.QueryRowContext(r.Context(), countSQL, args...).Scan(&total); err != nil {
        writeError(w, http.StatusInternalServerError, "读取考核记录失败")
        return
    }

    listSQL := `SELECT r.id, r.work_date, e.employee_code, e.name, COALESCE(d.name, ''), ws.start_at, ws.end_at,
 r.work_standard_seconds, r.break_seconds, r.need_reason, r.reason, r.violations_json
FROM work_session_reviews r
JOIN employees e ON r.employee_id = e.id
LEFT JOIN departments d ON e.department_id = d.id
LEFT JOIN work_sessions ws ON r.work_session_id = ws.id ` + where + ` ORDER BY r.created_at DESC LIMIT ? OFFSET ?`

    args = append(args, pageSize, (page-1)*pageSize)
    rows, err := h.DB.QueryContext(r.Context(), listSQL, args...)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "读取考核记录失败")
        return
    }
    defer rows.Close()

    items := make([]WorkSessionReviewListItem, 0)
    for rows.Next() {
        var id int64
        var workDate string
        var code string
        var name string
        var dept string
        var startAt sql.NullTime
        var endAt sql.NullTime
        var workSeconds int64
        var breakSeconds int64
        var needReason int
        var reason sql.NullString
        var violationsJSON string

        if err := rows.Scan(&id, &workDate, &code, &name, &dept, &startAt, &endAt, &workSeconds, &breakSeconds, &needReason, &reason, &violationsJSON); err != nil {
            writeError(w, http.StatusInternalServerError, "读取考核记录失败")
            return
        }

        summary := buildViolationSummary(violationsJSON)
        reasonStatus := "无需补录"
        if needReason > 0 {
            if reason.Valid && strings.TrimSpace(reason.String) != "" {
                reasonStatus = "已补录"
            } else {
                reasonStatus = "未补录"
            }
        }

        startText := "-"
        if startAt.Valid {
            startText = formatTime(startAt.Time)
        }
        endText := "-"
        if endAt.Valid {
            endText = formatTime(endAt.Time)
        }

        items = append(items, WorkSessionReviewListItem{
            ID:               id,
            WorkDate:         workDate,
            EmployeeCode:     code,
            Name:             name,
            Department:       dept,
            StartAt:          startText,
            EndAt:            endText,
            WorkStandard:     formatDuration(workSeconds),
            BreakDuration:    formatDuration(breakSeconds),
            ViolationSummary: summary,
            ReasonStatus:     reasonStatus,
        })
    }

    writeJSON(w, http.StatusOK, WorkSessionReviewListResponse{
        Total: total,
        Items: items,
    })
}

func (h *Handler) WorkSessionReviewDetail(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
        return
    }
    if h.DB == nil {
        writeError(w, http.StatusInternalServerError, "数据库未初始化")
        return
    }

    id := parseInt64(r.URL.Query().Get("id"))
    if id <= 0 {
        writeError(w, http.StatusBadRequest, "参数错误")
        return
    }

    query := `SELECT r.id, r.work_date, e.employee_code, e.name, COALESCE(d.name, ''), ws.start_at, ws.end_at,
 r.work_standard_seconds, r.break_seconds, r.need_reason, r.reason, r.violations_json
FROM work_session_reviews r
JOIN employees e ON r.employee_id = e.id
LEFT JOIN departments d ON e.department_id = d.id
LEFT JOIN work_sessions ws ON r.work_session_id = ws.id
WHERE r.id = ?`

    var recordID int64
    var workDate string
    var code string
    var name string
    var dept string
    var startAt sql.NullTime
    var endAt sql.NullTime
    var workSeconds int64
    var breakSeconds int64
    var needReason int
    var reason sql.NullString
    var violationsJSON string

    if err := h.DB.QueryRowContext(r.Context(), query, id).Scan(&recordID, &workDate, &code, &name, &dept, &startAt, &endAt, &workSeconds, &breakSeconds, &needReason, &reason, &violationsJSON); err != nil {
        if err == sql.ErrNoRows {
            writeError(w, http.StatusNotFound, "记录不存在")
        } else {
            writeError(w, http.StatusInternalServerError, "读取详情失败")
        }
        return
    }

    payload := WorkSessionReviewPayload{}
    _ = json.Unmarshal([]byte(violationsJSON), &payload)

    reasonStatus := "无需补录"
    reasonText := ""
    if needReason > 0 {
        if reason.Valid && strings.TrimSpace(reason.String) != "" {
            reasonStatus = "已补录"
            reasonText = reason.String
        } else {
            reasonStatus = "未补录"
        }
    }

    startText := "-"
    if startAt.Valid {
        startText = formatTime(startAt.Time)
    }
    endText := "-"
    if endAt.Valid {
        endText = formatTime(endAt.Time)
    }

    totals := map[string]string{}
    for key, value := range payload.StatusTotals {
        totals[key] = formatDuration(value)
    }

    writeJSON(w, http.StatusOK, WorkSessionReviewDetail{
        ID:            recordID,
        WorkDate:      workDate,
        EmployeeCode:  code,
        Name:          name,
        Department:    dept,
        StartAt:       startText,
        EndAt:         endText,
        WorkStandard:  formatDuration(workSeconds),
        BreakDuration: formatDuration(breakSeconds),
        ReasonStatus:  reasonStatus,
        Reason:        reasonText,
        Violations:    payload.Violations,
        StatusTotals:  totals,
    })
}

func buildViolationSummary(raw string) string {
    if raw == "" {
        return "-"
    }
    payload := WorkSessionReviewPayload{}
    if err := json.Unmarshal([]byte(raw), &payload); err != nil {
        return "-"
    }
    if len(payload.Violations) == 0 {
        return "-"
    }
    parts := make([]string, 0, len(payload.Violations))
    for _, v := range payload.Violations {
        if strings.TrimSpace(v.Message) == "" {
            continue
        }
        parts = append(parts, v.Message)
    }
    if len(parts) == 0 {
        return "-"
    }
    if len(parts) > 3 {
        return strings.Join(parts[:3], "；") + " 等"
    }
    return strings.Join(parts, "；")
}

func parseInt(value string, fallback int) int {
    if value == "" {
        return fallback
    }
    n, err := strconv.Atoi(value)
    if err != nil {
        return fallback
    }
    return n
}
