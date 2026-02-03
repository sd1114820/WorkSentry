package handlers

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "strconv"
    "strings"
    "time"
    "unicode/utf8"

    "worksentry/internal/db/sqlc"
)

const checkoutTextMaxLength = 1000

func (h *Handler) handleCheckoutOnWorkEnd(ctx context.Context, employee sqlc.Employee, payload ClientReportRequest, now time.Time) (bool, error) {
    if !employee.DepartmentID.Valid || employee.DepartmentID.Int64 <= 0 {
        return false, nil
    }

    template, err := h.Queries.GetEnabledCheckoutTemplateByDepartment(ctx, employee.DepartmentID.Int64)
    if err == sql.ErrNoRows {
        return false, nil
    }
    if err != nil {
        return true, fmt.Errorf("读取下班模板失败")
    }

    if payload.Checkout == nil {
        return true, fmt.Errorf("请填写下班信息")
    }
    if payload.Checkout.TemplateID != template.ID {
        return true, fmt.Errorf("下班模板已更新，请刷新后重试")
    }

    fields, err := h.Queries.ListCheckoutFieldsByTemplate(ctx, template.ID)
    if err != nil {
        return true, fmt.Errorf("读取下班字段失败")
    }

    snapshot := buildCheckoutSnapshot(template, fields)
    cleaned, err := validateCheckoutData(snapshot.Fields, payload.Checkout.Data)
    if err != nil {
        return true, err
    }

    if h.DB != nil {
        tx, err := h.DB.BeginTx(ctx, nil)
        if err != nil {
            return true, fmt.Errorf("提交下班信息失败")
        }
        qtx := h.Queries.WithTx(tx)
        session, err := qtx.GetOpenWorkSessionByEmployee(ctx, employee.ID)
        if err == sql.ErrNoRows {
            _ = tx.Rollback()
            return true, fmt.Errorf("未找到上班记录")
        }
        if err != nil {
            _ = tx.Rollback()
            return true, fmt.Errorf("读取上班记录失败")
        }
        if _, err := qtx.GetWorkSessionCheckoutBySessionID(ctx, session.ID); err == nil {
            _ = tx.Rollback()
            return true, fmt.Errorf("下班录入已提交")
        } else if err != sql.ErrNoRows {
            _ = tx.Rollback()
            return true, fmt.Errorf("读取下班记录失败")
        }

        snapshotJSON, err := json.Marshal(snapshot)
        if err != nil {
            _ = tx.Rollback()
            return true, fmt.Errorf("生成下班快照失败")
        }
        dataJSON, err := json.Marshal(cleaned)
        if err != nil {
            _ = tx.Rollback()
            return true, fmt.Errorf("生成下班数据失败")
        }

        if err := qtx.CreateWorkSessionCheckout(ctx, sqlc.CreateWorkSessionCheckoutParams{
            WorkSessionID:       session.ID,
            TemplateID:          template.ID,
            TemplateSnapshotJSON: string(snapshotJSON),
            DataJSON:            string(dataJSON),
        }); err != nil {
            _ = tx.Rollback()
            return true, fmt.Errorf("保存下班录入失败")
        }

        if err := qtx.CloseWorkSession(ctx, sqlc.CloseWorkSessionParams{
            EmployeeID: employee.ID,
            EndAt:      now,
        }); err != nil {
            _ = tx.Rollback()
            return true, fmt.Errorf("写入下班记录失败")
        }

        if err := tx.Commit(); err != nil {
            return true, fmt.Errorf("提交下班信息失败")
        }
        return true, nil
    }

    session, err := h.Queries.GetOpenWorkSessionByEmployee(ctx, employee.ID)
    if err == sql.ErrNoRows {
        return true, fmt.Errorf("未找到上班记录")
    }
    if err != nil {
        return true, fmt.Errorf("读取上班记录失败")
    }
    if _, err := h.Queries.GetWorkSessionCheckoutBySessionID(ctx, session.ID); err == nil {
        return true, fmt.Errorf("下班录入已提交")
    } else if err != sql.ErrNoRows {
        return true, fmt.Errorf("读取下班记录失败")
    }

    snapshotJSON, err := json.Marshal(snapshot)
    if err != nil {
        return true, fmt.Errorf("生成下班快照失败")
    }
    dataJSON, err := json.Marshal(cleaned)
    if err != nil {
        return true, fmt.Errorf("生成下班数据失败")
    }

    if err := h.Queries.CreateWorkSessionCheckout(ctx, sqlc.CreateWorkSessionCheckoutParams{
        WorkSessionID:       session.ID,
        TemplateID:          template.ID,
        TemplateSnapshotJSON: string(snapshotJSON),
        DataJSON:            string(dataJSON),
    }); err != nil {
        return true, fmt.Errorf("保存下班录入失败")
    }

    if err := h.Queries.CloseWorkSession(ctx, sqlc.CloseWorkSessionParams{
        EmployeeID: employee.ID,
        EndAt:      now,
    }); err != nil {
        return true, fmt.Errorf("写入下班记录失败")
    }

    return true, nil
}

func buildCheckoutSnapshot(template sqlc.CheckoutTemplate, fields []sqlc.CheckoutField) CheckoutTemplateSnapshot {
    snapshots := make([]CheckoutFieldSnapshot, 0, len(fields))
    for _, field := range fields {
        if !field.Enabled {
            continue
        }
        snapshots = append(snapshots, CheckoutFieldSnapshot{
            ID:       field.ID,
            Name:     field.NameZh,
            Type:     field.Type,
            Required: field.Required,
            Options:  parseOptions(field.OptionsZhJSON),
        })
    }

    return CheckoutTemplateSnapshot{
        TemplateID: template.ID,
        Name:       template.NameZh,
        Fields:     snapshots,
    }
}

func validateCheckoutData(fields []CheckoutFieldSnapshot, data map[string]string) (map[string]string, error) {
    if data == nil {
        data = map[string]string{}
    }
    cleaned := make(map[string]string, len(fields))
    for _, field := range fields {
        key := strconv.FormatInt(field.ID, 10)
        value := strings.TrimSpace(data[key])

        if value == "" {
            if field.Required {
                return nil, fmt.Errorf("请填写%s", field.Name)
            }
            continue
        }

        switch field.Type {
        case "text":
            if utf8.RuneCountInString(value) > checkoutTextMaxLength {
                return nil, fmt.Errorf("%s长度不能超过%d", field.Name, checkoutTextMaxLength)
            }
        case "number":
            if _, err := strconv.ParseFloat(value, 64); err != nil {
                return nil, fmt.Errorf("%s需填写数字", field.Name)
            }
        case "select":
            if !optionExists(field.Options, value) {
                return nil, fmt.Errorf("%s选项无效", field.Name)
            }
        default:
            return nil, fmt.Errorf("%s类型无效", field.Name)
        }

        cleaned[key] = value
    }
    return cleaned, nil
}

func optionExists(options []string, value string) bool {
    for _, opt := range options {
        if strings.TrimSpace(opt) == value {
            return true
        }
    }
    return false
}
