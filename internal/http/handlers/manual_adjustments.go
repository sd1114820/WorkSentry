package handlers

import (
	"database/sql"
	"net/http"
	"sort"
	"strconv"
	"time"

	"worksentry/internal/db/sqlc"
)

type ManualAdjustmentPayload struct {
	ID           int64  `json:"id"`
	EmployeeCode string `json:"employeeCode"`
	StartAt      string `json:"startAt"`
	EndAt        string `json:"endAt"`
	Reason       string `json:"reason"`
	Note         string `json:"note"`
}

type ManualAdjustmentView struct {
	ID           int64  `json:"id"`
	EmployeeCode string `json:"employeeCode"`
	Name         string `json:"name"`
	Department   string `json:"department"`
	StartAt      string `json:"startAt"`
	EndAt        string `json:"endAt"`
	Reason       string `json:"reason"`
	Note         string `json:"note"`
	Status       string `json:"status"`
	StatusLabel  string `json:"statusLabel"`
	CreatedAt    string `json:"createdAt"`
}

func (h *Handler) ManualAdjustments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listManualAdjustments(w, r)
	case http.MethodPost:
		h.createManualAdjustment(w, r)
	case http.MethodPut:
		h.updateManualAdjustment(w, r)
	case http.MethodDelete:
		h.revokeManualAdjustment(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
	}
}

func (h *Handler) listManualAdjustments(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	params := sqlc.ListManualAdjustmentsParams{
		Column1: date,
		StartAt: time.Time{},
	}
	if date != "" {
		parsed, err := parseDate(date)
		if err != nil {
			writeError(w, http.StatusBadRequest, "日期格式错误")
			return
		}
		params.StartAt = parsed
	}

	items, err := h.Queries.ListManualAdjustments(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取补录失败")
		return
	}

	views := make([]ManualAdjustmentView, 0, len(items))
	for _, item := range items {
		views = append(views, ManualAdjustmentView{
			ID:           item.ID,
			EmployeeCode: item.EmployeeCode,
			Name:         item.Name,
			Department:   nullString(item.DepartmentName),
			StartAt:      formatTime(item.StartAt),
			EndAt:        formatTime(item.EndAt),
			Reason:       item.Reason,
			Note:         item.Note,
			Status:       string(item.Status),
			StatusLabel:  manualStatusLabel(string(item.Status)),
			CreatedAt:    formatTime(item.CreatedAt),
		})
	}

	writeJSON(w, http.StatusOK, views)
}

func (h *Handler) createManualAdjustment(w http.ResponseWriter, r *http.Request) {
	var payload ManualAdjustmentPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "参数格式错误")
		return
	}
	if payload.EmployeeCode == "" || payload.StartAt == "" || payload.EndAt == "" || payload.Reason == "" || payload.Note == "" {
		writeError(w, http.StatusBadRequest, "工号、时间、原因、备注不能为空")
		return
	}
	startAt, err := parseDateTime(payload.StartAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "开始时间格式错误")
		return
	}
	endAt, err := parseDateTime(payload.EndAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "结束时间格式错误")
		return
	}
	if !endAt.After(startAt) {
		writeError(w, http.StatusBadRequest, "结束时间必须大于开始时间")
		return
	}

	employee, err := h.Queries.GetEmployeeByCode(r.Context(), payload.EmployeeCode)
	if err != nil {
		writeError(w, http.StatusNotFound, "员工不存在")
		return
	}

	if err := h.validateManualRange(r, employee.ID, startAt, endAt); err != "" {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	operatorID := adminIDFromRequest(r)
	result, err := h.Queries.CreateManualAdjustment(r.Context(), sqlc.CreateManualAdjustmentParams{
		EmployeeID: employee.ID,
		StartAt:    startAt,
		EndAt:      endAt,
		Reason:     payload.Reason,
		Note:       payload.Note,
		OperatorID: operatorID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "补录失败")
		return
	}

	id, err := result.LastInsertId()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "补录失败")
		return
	}

	h.applyManualStats(r, employee.ID, startAt, endAt, true)
	h.createSegmentOnly(r, employee.ID, startAt, endAt, "work", "补录", "manual")

	h.logAudit(r, "create_manual_adjustment", "manual_adjustment", sql.NullInt64{Int64: id, Valid: true}, payload)
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}
func (h *Handler) updateManualAdjustment(w http.ResponseWriter, r *http.Request) {
	var payload ManualAdjustmentPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "参数格式错误")
		return
	}
	if payload.ID <= 0 {
		writeError(w, http.StatusBadRequest, "补录编号不能为空")
		return
	}
	if payload.Reason == "" || payload.Note == "" {
		writeError(w, http.StatusBadRequest, "原因与备注不能为空")
		return
	}
	startAt, err := parseDateTime(payload.StartAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "开始时间格式错误")
		return
	}
	endAt, err := parseDateTime(payload.EndAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "结束时间格式错误")
		return
	}
	if !endAt.After(startAt) {
		writeError(w, http.StatusBadRequest, "结束时间必须大于开始时间")
		return
	}

	oldItem, err := h.Queries.GetManualAdjustment(r.Context(), payload.ID)
	if err != nil {
		writeError(w, http.StatusNotFound, "补录不存在")
		return
	}
	if oldItem.Status != sqlc.ManualAdjustmentsStatusActive {
		writeError(w, http.StatusBadRequest, "补录已撤销")
		return
	}

	if err := h.validateManualRange(r, oldItem.EmployeeID, startAt, endAt); err != "" {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := h.Queries.UpdateManualAdjustment(r.Context(), sqlc.UpdateManualAdjustmentParams{
		ID:      payload.ID,
		StartAt: startAt,
		EndAt:   endAt,
		Reason:  payload.Reason,
		Note:    payload.Note,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "更新补录失败")
		return
	}

	_ = h.Queries.DeleteManualSegment(r.Context(), sqlc.DeleteManualSegmentParams{EmployeeID: oldItem.EmployeeID, StartAt: oldItem.StartAt, EndAt: oldItem.EndAt})
	h.applyManualStats(r, oldItem.EmployeeID, oldItem.StartAt, oldItem.EndAt, false)
	h.applyManualStats(r, oldItem.EmployeeID, startAt, endAt, true)
	h.createSegmentOnly(r, oldItem.EmployeeID, startAt, endAt, "work", "补录", "manual")

	h.logAudit(r, "update_manual_adjustment", "manual_adjustment", sql.NullInt64{Int64: payload.ID, Valid: true}, payload)
	writeJSON(w, http.StatusOK, map[string]string{"message": "更新成功"})
}

func (h *Handler) revokeManualAdjustment(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "补录编号无效")
		return
	}
	item, err := h.Queries.GetManualAdjustment(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "补录不存在")
		return
	}
	if item.Status != sqlc.ManualAdjustmentsStatusActive {
		writeError(w, http.StatusBadRequest, "补录已撤销")
		return
	}

	if err := h.Queries.RevokeManualAdjustment(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "撤销失败")
		return
	}

	_ = h.Queries.DeleteManualSegment(r.Context(), sqlc.DeleteManualSegmentParams{EmployeeID: item.EmployeeID, StartAt: item.StartAt, EndAt: item.EndAt})
	h.applyManualStats(r, item.EmployeeID, item.StartAt, item.EndAt, false)

	h.logAudit(r, "revoke_manual_adjustment", "manual_adjustment", sql.NullInt64{Int64: id, Valid: true}, nil)
	writeJSON(w, http.StatusOK, map[string]string{"message": "已撤销"})
}

func manualStatusLabel(status string) string {
	switch status {
	case "active":
		return "生效"
	case "revoked":
		return "已撤销"
	default:
		return "未知"
	}
}

func (h *Handler) validateManualRange(r *http.Request, employeeID int64, startAt time.Time, endAt time.Time) string {
	segments, err := h.Queries.ListOfflineSegmentsByEmployeeAndRange(r.Context(), sqlc.ListOfflineSegmentsByEmployeeAndRangeParams{EmployeeID: employeeID, StartAt: endAt, EndAt: startAt})
	if err != nil {
		return "离线段校验失败"
	}
	if len(segments) == 0 {
		return "补录时间必须完全覆盖在离线段内"
	}

	sort.Slice(segments, func(i, j int) bool { return segments[i].StartAt.Before(segments[j].StartAt) })
	cursor := startAt
	for _, seg := range segments {
		if seg.StartAt.After(cursor) {
			return "补录时间必须完全覆盖在离线段内"
		}
		if seg.EndAt.After(cursor) {
			cursor = seg.EndAt
		}
		if !cursor.Before(endAt) {
			break
		}
	}
	if cursor.Before(endAt) {
		return "补录时间必须完全覆盖在离线段内"
	}

	overlap, err := h.Queries.CountNonOfflineSegmentsOverlap(r.Context(), sqlc.CountNonOfflineSegmentsOverlapParams{EmployeeID: employeeID, StartAt: endAt, EndAt: startAt})
	if err != nil {
		return "时间段校验失败"
	}
	if overlap > 0 {
		return "补录时间与已有系统数据冲突"
	}
	return ""
}

func (h *Handler) applyManualStats(r *http.Request, employeeID int64, startAt time.Time, endAt time.Time, add bool) {
	for _, part := range splitByDay(startAt, endAt) {
		inc := buildDailyStatIncrement("work", part.Seconds)
		sign := int32(1)
		if !add {
			sign = -1
		}
		_ = h.Queries.AddDailyStats(r.Context(), sqlc.AddDailyStatsParams{
			StatDate:          part.Date,
			EmployeeID:        employeeID,
			WorkSeconds:       inc.Work * sign,
			NormalSeconds:     0,
			FishSeconds:       0,
			IdleSeconds:       0,
			OfflineSeconds:    int32(part.Seconds) * -sign,
			AttendanceSeconds: inc.Attendance * sign,
			EffectiveSeconds:  inc.Effective * sign,
		})
	}
}
