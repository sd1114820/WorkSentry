package handlers

import (
    "bufio"
    "database/sql"
    "encoding/json"
    "net/http"
    "os"
    "path/filepath"
    "sort"
    "strings"
    "sync"
    "time"

    "worksentry/internal/db/sqlc"
)

var clientErrorFileLock sync.Mutex

type ClientErrorReportRequest struct {
    OccurredAt     string            `json:"occurredAt"`
    ErrorType      string            `json:"errorType"`
    ExceptionType  string            `json:"exceptionType"`
    Message        string            `json:"message"`
    StackTrace     string            `json:"stackTrace"`
    ClientVersion  string            `json:"clientVersion"`
    Context        map[string]string `json:"context"`
}

type ClientErrorReportResponse struct {
    Accepted bool `json:"accepted"`
}

type ClientErrorRecord struct {
    ID            string            `json:"id"`
    ReceivedAt    string            `json:"receivedAt"`
    OccurredAt    string            `json:"occurredAt"`
    EmployeeCode  string            `json:"employeeCode"`
    Name          string            `json:"name"`
    Department    string            `json:"department"`
    ErrorType     string            `json:"errorType"`
    ExceptionType string            `json:"exceptionType"`
    Message       string            `json:"message"`
    StackTrace    string            `json:"stackTrace"`
    ClientVersion string            `json:"clientVersion"`
    IP            string            `json:"ip"`
    Context       map[string]string `json:"context"`
}

func (h *Handler) ClientError(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
        return
    }

    token := readBearerToken(r)
    if token == "" {
        writeErrorWithCode(w, http.StatusUnauthorized, "token_missing", "缺少令牌")
        return
    }

    clientToken, err := h.Queries.GetToken(r.Context(), token)
    if err == sql.ErrNoRows {
        writeErrorWithCode(w, http.StatusUnauthorized, "token_invalid", "令牌无效")
        return
    }
    if err != nil {
        writeErrorWithCode(w, http.StatusInternalServerError, "token_validation_failed", "令牌校验失败")
        return
    }
    if clientToken.Revoked {
        writeErrorWithCode(w, http.StatusUnauthorized, "token_revoked", "令牌已失效")
        return
    }
    if clientToken.ExpiresAt.Valid && clientToken.ExpiresAt.Time.Before(time.Now()) {
        writeErrorWithCode(w, http.StatusUnauthorized, "token_expired", "令牌已过期")
        return
    }

    var payload ClientErrorReportRequest
    if err := decodeJSON(r, &payload); err != nil {
        writeError(w, http.StatusBadRequest, "参数格式错误")
        return
    }

    now := time.Now()
    occurredAt := now
    if strings.TrimSpace(payload.OccurredAt) != "" {
        if parsed, parseErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(payload.OccurredAt)); parseErr == nil {
            occurredAt = parsed.In(time.Local)
        }
    }

    var employeeCode string
    var name string
    var departmentName sql.NullString
    if h.DB != nil {
        _ = h.DB.QueryRowContext(r.Context(), `SELECT e.employee_code, e.name, d.name AS department_name
FROM employees e
LEFT JOIN departments d ON e.department_id = d.id
WHERE e.id = ?`, clientToken.EmployeeID).Scan(&employeeCode, &name, &departmentName)
    } else {
        employee, empErr := h.Queries.GetEmployeeByID(r.Context(), clientToken.EmployeeID)
        if empErr == nil {
            employeeCode = employee.EmployeeCode
            name = employee.Name
        }
    }

    id, _ := generateToken()

    record := ClientErrorRecord{
        ID:            id,
        ReceivedAt:    formatTime(now),
        OccurredAt:    formatTime(occurredAt),
        EmployeeCode:  employeeCode,
        Name:          name,
        Department:    nullString(departmentName),
        ErrorType:     strings.TrimSpace(payload.ErrorType),
        ExceptionType: strings.TrimSpace(payload.ExceptionType),
        Message:       strings.TrimSpace(payload.Message),
        StackTrace:    payload.StackTrace,
        ClientVersion: strings.TrimSpace(payload.ClientVersion),
        IP:            clientIP(r),
        Context:       payload.Context,
    }

    if record.ErrorType == "" {
        record.ErrorType = "unknown"
    }

    dir := filepath.Join("data", "client-errors")
    if err := os.MkdirAll(dir, 0o755); err != nil {
        writeErrorWithCode(w, http.StatusInternalServerError, "server_error", "写入失败")
        return
    }

    filename := filepath.Join(dir, now.Format("2006-01-02")+".jsonl")

    b, err := json.Marshal(record)
    if err != nil {
        writeErrorWithCode(w, http.StatusInternalServerError, "server_error", "写入失败")
        return
    }

    clientErrorFileLock.Lock()
    defer clientErrorFileLock.Unlock()

    f, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
    if err != nil {
        writeErrorWithCode(w, http.StatusInternalServerError, "server_error", "写入失败")
        return
    }
    defer f.Close()

    if _, err := f.Write(append(b, '\n')); err != nil {
        writeErrorWithCode(w, http.StatusInternalServerError, "server_error", "写入失败")
        return
    }

    _ = h.Queries.UpdateTokenLastSeen(r.Context(), sqlc.UpdateTokenLastSeenParams{
        LastSeen: sql.NullTime{Time: now, Valid: true},
        Token:    token,
    })

    writeJSON(w, http.StatusOK, ClientErrorReportResponse{Accepted: true})
}

func (h *Handler) ClientErrors(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
        return
    }

    dateValue := strings.TrimSpace(r.URL.Query().Get("date"))
    if dateValue == "" {
        dateValue = time.Now().Format("2006-01-02")
    }

    date, err := parseDate(dateValue)
    if err != nil {
        writeError(w, http.StatusBadRequest, "日期格式错误")
        return
    }

    employeeCode := strings.TrimSpace(r.URL.Query().Get("employeeCode"))
    limit := int64(200)

    dir := filepath.Join("data", "client-errors")
    filename := filepath.Join(dir, date.Format("2006-01-02")+".jsonl")

    if _, err := os.Stat(filename); err != nil {
        writeJSON(w, http.StatusOK, []ClientErrorRecord{})
        return
    }

    file, err := os.Open(filename)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "读取失败")
        return
    }
    defer file.Close()

    scanner := bufio.NewScanner(file)
    scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

    items := make([]ClientErrorRecord, 0, 64)
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        if line == "" {
            continue
        }
        var item ClientErrorRecord
        if err := json.Unmarshal([]byte(line), &item); err != nil {
            continue
        }
        if employeeCode != "" && item.EmployeeCode != employeeCode {
            continue
        }
        items = append(items, item)
    }
    if err := scanner.Err(); err != nil {
        writeError(w, http.StatusInternalServerError, "读取客户端错误日志失败")
        return
    }

    sort.Slice(items, func(i, j int) bool {
        return items[i].ReceivedAt > items[j].ReceivedAt
    })

    if int64(len(items)) > limit {
        items = items[:limit]
    }

    writeJSON(w, http.StatusOK, items)
}
