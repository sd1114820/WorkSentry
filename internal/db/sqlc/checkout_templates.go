package sqlc

import (
	"context"
	"database/sql"
	"time"
)

type CheckoutTemplate struct {
	ID           int64     `json:"id"`
	DepartmentID int64     `json:"department_id"`
	NameZh       string    `json:"name_zh"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type CheckoutTemplateWithCount struct {
	CheckoutTemplate
	FieldCount int64 `json:"field_count"`
}

const listCheckoutTemplatesByDepartment = `SELECT t.id, t.department_id, t.name_zh, t.enabled, t.created_at, t.updated_at,
       COUNT(f.id) AS field_count
FROM checkout_templates t
LEFT JOIN checkout_fields f ON f.template_id = t.id
WHERE t.department_id = ?
GROUP BY t.id
ORDER BY t.updated_at DESC, t.id DESC`

func (q *Queries) ListCheckoutTemplatesByDepartment(ctx context.Context, departmentID int64) ([]CheckoutTemplateWithCount, error) {
	rows, err := q.db.QueryContext(ctx, listCheckoutTemplatesByDepartment, departmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []CheckoutTemplateWithCount
	for rows.Next() {
		var item CheckoutTemplateWithCount
		if err := rows.Scan(
			&item.ID,
			&item.DepartmentID,
			&item.NameZh,
			&item.Enabled,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.FieldCount,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getCheckoutTemplateByID = `SELECT id, department_id, name_zh, enabled, created_at, updated_at
FROM checkout_templates
WHERE id = ?`

func (q *Queries) GetCheckoutTemplateByID(ctx context.Context, id int64) (CheckoutTemplate, error) {
	row := q.db.QueryRowContext(ctx, getCheckoutTemplateByID, id)
	var item CheckoutTemplate
	err := row.Scan(
		&item.ID,
		&item.DepartmentID,
		&item.NameZh,
		&item.Enabled,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	return item, err
}

const getEnabledCheckoutTemplateByDepartment = `SELECT id, department_id, name_zh, enabled, created_at, updated_at
FROM checkout_templates
WHERE department_id = ? AND enabled = 1
ORDER BY updated_at DESC, id DESC
LIMIT 1`

func (q *Queries) GetEnabledCheckoutTemplateByDepartment(ctx context.Context, departmentID int64) (CheckoutTemplate, error) {
	row := q.db.QueryRowContext(ctx, getEnabledCheckoutTemplateByDepartment, departmentID)
	var item CheckoutTemplate
	err := row.Scan(
		&item.ID,
		&item.DepartmentID,
		&item.NameZh,
		&item.Enabled,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	return item, err
}

const createCheckoutTemplate = `INSERT INTO checkout_templates (department_id, name_zh, enabled)
VALUES (?, ?, ?)`

type CreateCheckoutTemplateParams struct {
	DepartmentID int64
	NameZh       string
	Enabled      bool
}

func (q *Queries) CreateCheckoutTemplate(ctx context.Context, arg CreateCheckoutTemplateParams) (int64, error) {
	result, err := q.db.ExecContext(ctx, createCheckoutTemplate, arg.DepartmentID, arg.NameZh, arg.Enabled)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

const updateCheckoutTemplate = `UPDATE checkout_templates
SET name_zh = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?`

type UpdateCheckoutTemplateParams struct {
	ID      int64
	NameZh  string
	Enabled bool
}

func (q *Queries) UpdateCheckoutTemplate(ctx context.Context, arg UpdateCheckoutTemplateParams) error {
	_, err := q.db.ExecContext(ctx, updateCheckoutTemplate, arg.NameZh, arg.Enabled, arg.ID)
	return err
}

const deleteCheckoutTemplate = `DELETE FROM checkout_templates WHERE id = ?`

func (q *Queries) DeleteCheckoutTemplate(ctx context.Context, id int64) error {
	_, err := q.db.ExecContext(ctx, deleteCheckoutTemplate, id)
	return err
}

const disableOtherCheckoutTemplates = `UPDATE checkout_templates
SET enabled = 0, updated_at = CURRENT_TIMESTAMP
WHERE department_id = ? AND id <> ? AND enabled = 1`

func (q *Queries) DisableOtherCheckoutTemplates(ctx context.Context, departmentID int64, keepID int64) error {
	_, err := q.db.ExecContext(ctx, disableOtherCheckoutTemplates, departmentID, keepID)
	return err
}

type CheckoutField struct {
	ID            int64          `json:"id"`
	TemplateID    int64          `json:"template_id"`
	NameZh        string         `json:"name_zh"`
	Type          string         `json:"type"`
	Required      bool           `json:"required"`
	SortOrder     int32          `json:"sort_order"`
	Enabled       bool           `json:"enabled"`
	OptionsZhJSON sql.NullString `json:"options_zh_json"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

const listCheckoutFieldsByTemplate = `SELECT id, template_id, name_zh, type, required, sort_order, enabled, options_zh_json, created_at, updated_at
FROM checkout_fields
WHERE template_id = ?
ORDER BY sort_order ASC, id ASC`

func (q *Queries) ListCheckoutFieldsByTemplate(ctx context.Context, templateID int64) ([]CheckoutField, error) {
	rows, err := q.db.QueryContext(ctx, listCheckoutFieldsByTemplate, templateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []CheckoutField
	for rows.Next() {
		var item CheckoutField
		if err := rows.Scan(
			&item.ID,
			&item.TemplateID,
			&item.NameZh,
			&item.Type,
			&item.Required,
			&item.SortOrder,
			&item.Enabled,
			&item.OptionsZhJSON,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const createCheckoutField = `INSERT INTO checkout_fields (template_id, name_zh, type, required, sort_order, enabled, options_zh_json)
VALUES (?, ?, ?, ?, ?, ?, ?)`

type CreateCheckoutFieldParams struct {
	TemplateID    int64
	NameZh        string
	Type          string
	Required      bool
	SortOrder     int32
	Enabled       bool
	OptionsZhJSON sql.NullString
}

func (q *Queries) CreateCheckoutField(ctx context.Context, arg CreateCheckoutFieldParams) (int64, error) {
	result, err := q.db.ExecContext(ctx, createCheckoutField, arg.TemplateID, arg.NameZh, arg.Type, arg.Required, arg.SortOrder, arg.Enabled, arg.OptionsZhJSON)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

const updateCheckoutField = `UPDATE checkout_fields
SET name_zh = ?, type = ?, required = ?, sort_order = ?, enabled = ?, options_zh_json = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?`

type UpdateCheckoutFieldParams struct {
	ID            int64
	NameZh        string
	Type          string
	Required      bool
	SortOrder     int32
	Enabled       bool
	OptionsZhJSON sql.NullString
}

func (q *Queries) UpdateCheckoutField(ctx context.Context, arg UpdateCheckoutFieldParams) error {
	_, err := q.db.ExecContext(ctx, updateCheckoutField, arg.NameZh, arg.Type, arg.Required, arg.SortOrder, arg.Enabled, arg.OptionsZhJSON, arg.ID)
	return err
}

const deleteCheckoutField = `DELETE FROM checkout_fields WHERE id = ?`

func (q *Queries) DeleteCheckoutField(ctx context.Context, id int64) error {
	_, err := q.db.ExecContext(ctx, deleteCheckoutField, id)
	return err
}

type WorkSessionCheckout struct {
	ID                   int64     `json:"id"`
	WorkSessionID        int64     `json:"work_session_id"`
	TemplateID           int64     `json:"template_id"`
	TemplateSnapshotJSON string    `json:"template_snapshot_json"`
	DataJSON             string    `json:"data_json"`
	CreatedAt            time.Time `json:"created_at"`
}

const getWorkSessionCheckoutBySessionID = `SELECT id, work_session_id, template_id, template_snapshot_json, data_json, created_at
FROM work_session_checkouts
WHERE work_session_id = ?
LIMIT 1`

func (q *Queries) GetWorkSessionCheckoutBySessionID(ctx context.Context, sessionID int64) (WorkSessionCheckout, error) {
	row := q.db.QueryRowContext(ctx, getWorkSessionCheckoutBySessionID, sessionID)
	var item WorkSessionCheckout
	err := row.Scan(&item.ID, &item.WorkSessionID, &item.TemplateID, &item.TemplateSnapshotJSON, &item.DataJSON, &item.CreatedAt)
	return item, err
}

const createWorkSessionCheckout = `INSERT INTO work_session_checkouts (work_session_id, template_id, template_snapshot_json, data_json)
VALUES (?, ?, ?, ?)`

type CreateWorkSessionCheckoutParams struct {
	WorkSessionID        int64
	TemplateID           int64
	TemplateSnapshotJSON string
	DataJSON             string
}

func (q *Queries) CreateWorkSessionCheckout(ctx context.Context, arg CreateWorkSessionCheckoutParams) error {
	_, err := q.db.ExecContext(ctx, createWorkSessionCheckout, arg.WorkSessionID, arg.TemplateID, arg.TemplateSnapshotJSON, arg.DataJSON)
	return err
}
