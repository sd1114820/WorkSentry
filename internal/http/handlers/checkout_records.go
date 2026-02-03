package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type CheckoutRecordView struct {
	ID           int64  `json:"id"`
	EmployeeCode string `json:"employeeCode"`
	Name         string `json:"name"`
	Department   string `json:"department"`
	StartAt      string `json:"startAt"`
	EndAt        string `json:"endAt"`
	TemplateName string `json:"templateName"`
	CreatedAt    string `json:"createdAt"`
	Summary      string `json:"summary"`
}

type CheckoutRecordListResponse struct {
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"pageSize"`
	Items    []CheckoutRecordView `json:"items"`
}

type CheckoutRecordFieldView struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

type CheckoutRecordDetailView struct {
	ID           int64                     `json:"id"`
	EmployeeCode string                    `json:"employeeCode"`
	Name         string                    `json:"name"`
	Department   string                    `json:"department"`
	StartAt      string                    `json:"startAt"`
	EndAt        string                    `json:"endAt"`
	TemplateName string                    `json:"templateName"`
	CreatedAt    string                    `json:"createdAt"`
	Fields       []CheckoutRecordFieldView `json:"fields"`
}

func (h *Handler) CheckoutRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
		return
	}
	if h.DB == nil {
		writeError(w, http.StatusInternalServerError, "数据库未就绪")
		return
	}

	startDate := strings.TrimSpace(r.URL.Query().Get("startDate"))
	endDate := strings.TrimSpace(r.URL.Query().Get("endDate"))
	if startDate == "" || endDate == "" {
		today := time.Now().Format("2006-01-02")
		if startDate == "" {
			startDate = today
		}
		if endDate == "" {
			endDate = today
		}
	}

	start, err := parseDate(startDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "开始日期格式错误")
		return
	}
	end, err := parseDate(endDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "结束日期格式错误")
		return
	}
	if end.Before(start) {
		writeError(w, http.StatusBadRequest, "结束日期不能早于开始日期")
		return
	}
	endExclusive := end.Add(24 * time.Hour)

	departmentID, _ := strconv.ParseInt(r.URL.Query().Get("departmentId"), 10, 64)
	templateID, _ := strconv.ParseInt(r.URL.Query().Get("templateId"), 10, 64)
	employeeKeyword := strings.TrimSpace(r.URL.Query().Get("employeeKeyword"))

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize

	whereClauses := []string{"c.created_at >= ?", "c.created_at < ?"}
	args := []any{start, endExclusive}

	if departmentID > 0 {
		whereClauses = append(whereClauses, "e.department_id = ?")
		args = append(args, departmentID)
	}
	if templateID > 0 {
		whereClauses = append(whereClauses, "c.template_id = ?")
		args = append(args, templateID)
	}
	if employeeKeyword != "" {
		whereClauses = append(whereClauses, "(e.employee_code LIKE ? OR e.name LIKE ?)")
		like := "%" + employeeKeyword + "%"
		args = append(args, like, like)
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	countSQL := "SELECT COUNT(1) FROM work_session_checkouts c " +
		"JOIN work_sessions ws ON ws.id = c.work_session_id " +
		"JOIN employees e ON e.id = ws.employee_id " +
		"LEFT JOIN departments d ON d.id = e.department_id " +
		"LEFT JOIN checkout_templates t ON t.id = c.template_id " +
		"WHERE " + whereSQL

	var total int64
	if err := h.DB.QueryRowContext(r.Context(), countSQL, args...).Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, "读取模板失败")
		return
	}

	listSQL := "SELECT c.id, e.employee_code, e.name, COALESCE(d.name, ''), ws.start_at, ws.end_at, COALESCE(t.name_zh, ''), c.created_at, c.template_snapshot_json, c.data_json " +
		"FROM work_session_checkouts c " +
		"JOIN work_sessions ws ON ws.id = c.work_session_id " +
		"JOIN employees e ON e.id = ws.employee_id " +
		"LEFT JOIN departments d ON d.id = e.department_id " +
		"LEFT JOIN checkout_templates t ON t.id = c.template_id " +
		"WHERE " + whereSQL + " ORDER BY c.created_at DESC LIMIT ? OFFSET ?"

	listArgs := append(append([]any{}, args...), pageSize, offset)
	rows, err := h.DB.QueryContext(r.Context(), listSQL, listArgs...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取模板失败")
		return
	}
	defer rows.Close()

	items := make([]CheckoutRecordView, 0)
	for rows.Next() {
		var (
			id           int64
			employeeCode string
			name         string
			department   string
			startAt      time.Time
			endAt        sql.NullTime
			templateName string
			createdAt    time.Time
			snapshotJSON string
			dataJSON     string
		)
		if err := rows.Scan(&id, &employeeCode, &name, &department, &startAt, &endAt, &templateName, &createdAt, &snapshotJSON, &dataJSON); err != nil {
			writeError(w, http.StatusInternalServerError, "读取模板失败")
			return
		}
		summary := buildCheckoutSummary(snapshotJSON, dataJSON)
		endAtText := ""
		if endAt.Valid {
			endAtText = formatTime(endAt.Time)
		}
		items = append(items, CheckoutRecordView{
			ID:           id,
			EmployeeCode: employeeCode,
			Name:         name,
			Department:   department,
			StartAt:      formatTime(startAt),
			EndAt:        endAtText,
			TemplateName: templateName,
			CreatedAt:    formatTime(createdAt),
			Summary:      summary,
		})
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "读取模板失败")
		return
	}

	writeJSON(w, http.StatusOK, CheckoutRecordListResponse{
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Items:    items,
	})
}

func (h *Handler) CheckoutRecordDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
		return
	}
	if h.DB == nil {
		writeError(w, http.StatusInternalServerError, "数据库未就绪")
		return
	}

	id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if id <= 0 {
		writeError(w, http.StatusBadRequest, "记录编号无效")
		return
	}

	query := "SELECT c.id, e.employee_code, e.name, COALESCE(d.name, ''), ws.start_at, ws.end_at, COALESCE(t.name_zh, ''), c.created_at, c.template_snapshot_json, c.data_json " +
		"FROM work_session_checkouts c " +
		"JOIN work_sessions ws ON ws.id = c.work_session_id " +
		"JOIN employees e ON e.id = ws.employee_id " +
		"LEFT JOIN departments d ON d.id = e.department_id " +
		"LEFT JOIN checkout_templates t ON t.id = c.template_id " +
		"WHERE c.id = ?"

	var (
		recordID     int64
		employeeCode string
		name         string
		department   string
		startAt      time.Time
		endAt        sql.NullTime
		templateName string
		createdAt    time.Time
		snapshotJSON string
		dataJSON     string
	)
	if err := h.DB.QueryRowContext(r.Context(), query, id).Scan(&recordID, &employeeCode, &name, &department, &startAt, &endAt, &templateName, &createdAt, &snapshotJSON, &dataJSON); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "记录不存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "读取模板失败")
		return
	}

	fields := buildCheckoutDetailFields(snapshotJSON, dataJSON)
	endAtText := ""
	if endAt.Valid {
		endAtText = formatTime(endAt.Time)
	}

	writeJSON(w, http.StatusOK, CheckoutRecordDetailView{
		ID:           recordID,
		EmployeeCode: employeeCode,
		Name:         name,
		Department:   department,
		StartAt:      formatTime(startAt),
		EndAt:        endAtText,
		TemplateName: templateName,
		CreatedAt:    formatTime(createdAt),
		Fields:       fields,
	})
}

func buildCheckoutSummary(snapshotJSON string, dataJSON string) string {
	snapshot := CheckoutTemplateSnapshot{}
	if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil {
		return "-"
	}
	if len(snapshot.Fields) == 0 {
		return "-"
	}
	data := map[string]string{}
	_ = json.Unmarshal([]byte(dataJSON), &data)
	parts := make([]string, 0, 2)
	for _, field := range snapshot.Fields {
		key := strconv.FormatInt(field.ID, 10)
		value := strings.TrimSpace(data[key])
		if value == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s：%s", field.Name, value))
		if len(parts) >= 2 {
			break
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " / ")
}

func buildCheckoutDetailFields(snapshotJSON string, dataJSON string) []CheckoutRecordFieldView {
	snapshot := CheckoutTemplateSnapshot{}
	if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil {
		return nil
	}
	data := map[string]string{}
	_ = json.Unmarshal([]byte(dataJSON), &data)
	fields := make([]CheckoutRecordFieldView, 0, len(snapshot.Fields))
	for _, field := range snapshot.Fields {
		key := strconv.FormatInt(field.ID, 10)
		value := strings.TrimSpace(data[key])
		fields = append(fields, CheckoutRecordFieldView{
			Name:  field.Name,
			Type:  field.Type,
			Value: value,
		})
	}
	return fields
}
