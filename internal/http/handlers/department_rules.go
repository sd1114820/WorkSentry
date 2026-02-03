package handlers

import (
    "context"
    "database/sql"
    "encoding/json"
    "net/http"
    "strings"
)

type DepartmentRuleView struct {
    TargetSeconds         int32 `json:"targetSeconds"`
    MaxBreakSeconds       int32 `json:"maxBreakSeconds"`
    MaxBreakCount         int32 `json:"maxBreakCount"`
    MaxBreakSingleSeconds int32 `json:"maxBreakSingleSeconds"`
}

type StatusThresholdView struct {
    StatusCode    string `json:"statusCode"`
    MinSeconds    int32  `json:"minSeconds"`
    MaxSeconds    int32  `json:"maxSeconds"`
    TriggerAction string `json:"triggerAction"`
    Enabled       bool   `json:"enabled"`
}

type DepartmentRuleResponse struct {
    DepartmentID int64                 `json:"departmentId"`
    Enabled      bool                  `json:"enabled"`
    Rule         DepartmentRuleView    `json:"rule"`
    Thresholds   []StatusThresholdView `json:"thresholds"`
}

type DepartmentRulePayload struct {
    DepartmentID int64                  `json:"departmentId"`
    Enabled      bool                   `json:"enabled"`
    Rule         DepartmentRuleView     `json:"rule"`
    Thresholds   []StatusThresholdView  `json:"thresholds"`
}

const (
    triggerShowOnly     = "show_only"
    triggerRequireReason = "require_reason"
)

func (h *Handler) DepartmentRules(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        h.getDepartmentRules(w, r)
    case http.MethodPut:
        h.saveDepartmentRules(w, r)
    default:
        writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
    }
}

func (h *Handler) getDepartmentRules(w http.ResponseWriter, r *http.Request) {
    if h.DB == nil {
        writeError(w, http.StatusInternalServerError, "数据库未初始化")
        return
    }

    departmentID := parseInt64(r.URL.Query().Get("departmentId"))
    if departmentID <= 0 {
        writeError(w, http.StatusBadRequest, "请选择部门")
        return
    }

    rule := DepartmentRuleView{}
    enabled := false

    row := h.DB.QueryRowContext(r.Context(), `SELECT target_seconds, max_break_seconds, max_break_count, IFNULL(max_break_single_seconds, 0)
FROM department_work_rules WHERE department_id = ?`, departmentID)
    if err := row.Scan(&rule.TargetSeconds, &rule.MaxBreakSeconds, &rule.MaxBreakCount, &rule.MaxBreakSingleSeconds); err == nil {
        enabled = true
    } else if err != sql.ErrNoRows {
        writeError(w, http.StatusInternalServerError, "读取部门规则失败")
        return
    }

    thresholds, err := h.listDepartmentThresholds(r.Context(), departmentID)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "读取阈值规则失败")
        return
    }
    if len(thresholds) > 0 {
        enabled = true
    }

    writeJSON(w, http.StatusOK, DepartmentRuleResponse{
        DepartmentID: departmentID,
        Enabled:      enabled,
        Rule:         rule,
        Thresholds:   thresholds,
    })
}

func (h *Handler) saveDepartmentRules(w http.ResponseWriter, r *http.Request) {
    if h.DB == nil {
        writeError(w, http.StatusInternalServerError, "数据库未初始化")
        return
    }

    var payload DepartmentRulePayload
    if err := decodeJSON(r, &payload); err != nil {
        writeError(w, http.StatusBadRequest, "参数格式错误")
        return
    }
    if payload.DepartmentID <= 0 {
        writeError(w, http.StatusBadRequest, "请选择部门")
        return
    }

    if payload.Rule.TargetSeconds < 0 || payload.Rule.MaxBreakSeconds < 0 || payload.Rule.MaxBreakCount < 0 || payload.Rule.MaxBreakSingleSeconds < 0 {
        writeError(w, http.StatusBadRequest, "时长配置不正确")
        return
    }

    for _, item := range payload.Thresholds {
        if !isValidStatusCode(item.StatusCode) {
            writeError(w, http.StatusBadRequest, "状态类型无效")
            return
        }
        if item.MinSeconds < 0 || item.MaxSeconds < 0 {
            writeError(w, http.StatusBadRequest, "阈值不能为负数")
            return
        }
        if item.TriggerAction != triggerShowOnly && item.TriggerAction != triggerRequireReason {
            writeError(w, http.StatusBadRequest, "触发处理无效")
            return
        }
    }

    tx, err := h.DB.BeginTx(r.Context(), nil)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "保存失败")
        return
    }

    if !payload.Enabled {
        if _, err := tx.ExecContext(r.Context(), "DELETE FROM department_work_rules WHERE department_id = ?", payload.DepartmentID); err != nil {
            _ = tx.Rollback()
            writeError(w, http.StatusInternalServerError, "更新规则失败")
            return
        }
        if _, err := tx.ExecContext(r.Context(), "DELETE FROM department_status_thresholds WHERE department_id = ?", payload.DepartmentID); err != nil {
            _ = tx.Rollback()
            writeError(w, http.StatusInternalServerError, "更新阈值失败")
            return
        }
    } else {
        _, err = tx.ExecContext(r.Context(), `INSERT INTO department_work_rules (department_id, target_seconds, max_break_seconds, max_break_count, max_break_single_seconds)
VALUES (?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE target_seconds = VALUES(target_seconds), max_break_seconds = VALUES(max_break_seconds), max_break_count = VALUES(max_break_count), max_break_single_seconds = VALUES(max_break_single_seconds)`,
            payload.DepartmentID, payload.Rule.TargetSeconds, payload.Rule.MaxBreakSeconds, payload.Rule.MaxBreakCount, nullIfZero(payload.Rule.MaxBreakSingleSeconds))
        if err != nil {
            _ = tx.Rollback()
            writeError(w, http.StatusInternalServerError, "保存规则失败")
            return
        }

        if _, err := tx.ExecContext(r.Context(), "DELETE FROM department_status_thresholds WHERE department_id = ?", payload.DepartmentID); err != nil {
            _ = tx.Rollback()
            writeError(w, http.StatusInternalServerError, "保存阈值失败")
            return
        }

        for _, item := range payload.Thresholds {
            _, err = tx.ExecContext(r.Context(), `INSERT INTO department_status_thresholds (department_id, status_code, min_seconds, max_seconds, trigger_action, enabled)
VALUES (?, ?, ?, ?, ?, ?)`,
                payload.DepartmentID,
                strings.TrimSpace(item.StatusCode),
                nullIfZero(item.MinSeconds),
                nullIfZero(item.MaxSeconds),
                item.TriggerAction,
                boolToTinyInt(item.Enabled),
            )
            if err != nil {
                _ = tx.Rollback()
                writeError(w, http.StatusInternalServerError, "保存阈值失败")
                return
            }
        }
    }

    if err := tx.Commit(); err != nil {
        writeError(w, http.StatusInternalServerError, "保存失败")
        return
    }

    h.logAudit(r, "update_department_rules", "departments", sql.NullInt64{Int64: payload.DepartmentID, Valid: true}, payload)
    writeJSON(w, http.StatusOK, map[string]string{"message": "ok"})
}

func (h *Handler) listDepartmentThresholds(ctx context.Context, departmentID int64) ([]StatusThresholdView, error) {
    rows, err := h.DB.QueryContext(ctx, `SELECT status_code, IFNULL(min_seconds, 0), IFNULL(max_seconds, 0), trigger_action, enabled
FROM department_status_thresholds WHERE department_id = ? ORDER BY id`, departmentID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var list []StatusThresholdView
    for rows.Next() {
        var statusCode string
        var minSeconds int32
        var maxSeconds int32
        var triggerAction string
        var enabled int
        if err := rows.Scan(&statusCode, &minSeconds, &maxSeconds, &triggerAction, &enabled); err != nil {
            return nil, err
        }
        list = append(list, StatusThresholdView{
            StatusCode:    statusCode,
            MinSeconds:    minSeconds,
            MaxSeconds:    maxSeconds,
            TriggerAction: triggerAction,
            Enabled:       enabled > 0,
        })
    }
    return list, rows.Err()
}

func isValidStatusCode(code string) bool {
    switch strings.TrimSpace(code) {
    case "work", "normal", "fish", "idle", "offline", "break":
        return true
    default:
        return false
    }
}

func boolToTinyInt(value bool) int {
    if value {
        return 1
    }
    return 0
}

func nullIfZero(value int32) interface{} {
    if value <= 0 {
        return nil
    }
    return value
}

func (h *Handler) writeJSONWithData(w http.ResponseWriter, status int, message string, code string, data any) {
    payload := map[string]any{
        "message": message,
    }
    if code != "" {
        payload["code"] = code
    }
    if data != nil {
        raw, err := json.Marshal(data)
        if err == nil {
            var parsed any
            if json.Unmarshal(raw, &parsed) == nil {
                payload["data"] = parsed
            } else {
                payload["data"] = data
            }
        } else {
            payload["data"] = data
        }
    }
    writeJSON(w, status, payload)
}






