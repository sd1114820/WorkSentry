package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"worksentry/internal/db/sqlc"
)

type ClientBindRequest struct {
	EmployeeCode  string `json:"employeeCode"`
	Fingerprint   string `json:"fingerprint"`
	ClientVersion string `json:"clientVersion"`
}

type ClientBindResponse struct {
	Token                    string `json:"token"`
	IdleThresholdSeconds     int32  `json:"idleThresholdSeconds"`
	HeartbeatIntervalSeconds int32  `json:"heartbeatIntervalSeconds"`
	OfflineThresholdSeconds  int32  `json:"offlineThresholdSeconds"`
	UpdatePolicy             int32  `json:"updatePolicy"`
	LatestVersion            string `json:"latestVersion"`
	UpdateURL                string `json:"updateUrl"`
	ServerTime               string `json:"serverTime"`
}

type ClientCheckoutPayload struct {
	TemplateID int64             `json:"templateId"`
	Data       map[string]string `json:"data"`
}

type ClientReportRequest struct {
	ProcessName   string                 `json:"processName"`
	WindowTitle   string                 `json:"windowTitle"`
	IdleSeconds   int32                  `json:"idleSeconds"`
	ClientVersion string                 `json:"clientVersion"`
	ReportType    string                 `json:"reportType"`
	Checkout      *ClientCheckoutPayload `json:"checkout"`
	Reason        string                 `json:"reason"`
}

type ClientReportResponse struct {
	IdleThresholdSeconds     int32  `json:"idleThresholdSeconds"`
	HeartbeatIntervalSeconds int32  `json:"heartbeatIntervalSeconds"`
	OfflineThresholdSeconds  int32  `json:"offlineThresholdSeconds"`
	UpdatePolicy             int32  `json:"updatePolicy"`
	LatestVersion            string `json:"latestVersion"`
	UpdateURL                string `json:"updateUrl"`
	ServerTime               string `json:"serverTime"`
}

func (h *Handler) ClientBind(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
		return
	}

	var payload ClientBindRequest
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "参数格式错误")
		return
	}
	payload.EmployeeCode = strings.TrimSpace(payload.EmployeeCode)
	payload.Fingerprint = strings.TrimSpace(payload.Fingerprint)

	if payload.EmployeeCode == "" || payload.Fingerprint == "" {
		writeError(w, http.StatusBadRequest, "工号与硬件指纹不能为空")
		return
	}

	employee, err := h.Queries.GetEmployeeByCode(r.Context(), payload.EmployeeCode)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "工号不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "查询员工失败")
		return
	}
	if !employee.Enabled {
		writeError(w, http.StatusForbidden, "员工已停用")
		return
	}

	if employee.FingerprintHash.Valid {
		if employee.FingerprintHash.String != payload.Fingerprint {
			writeError(w, http.StatusForbidden, "设备不匹配，请联系管理员解绑")
			return
		}
	} else {
		if err := h.Queries.UpdateEmployeeFingerprint(r.Context(), sqlc.UpdateEmployeeFingerprintParams{
			FingerprintHash: toNullString(payload.Fingerprint),
			ID:              employee.ID,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "绑定失败")
			return
		}
	}

	token, err := generateToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "生成令牌失败")
		return
	}

	now := time.Now()
	if err := h.Queries.CreateToken(r.Context(), sqlc.CreateTokenParams{
		Token:      token,
		EmployeeID: employee.ID,
		IssuedAt:   now,
		ExpiresAt:  sql.NullTime{Valid: false},
		LastSeen:   sql.NullTime{Time: now, Valid: true},
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "创建令牌失败")
		return
	}

	settings := h.getSettingsOrDefault(r)

	writeJSON(w, http.StatusOK, ClientBindResponse{
		Token:                    token,
		IdleThresholdSeconds:     settings.IdleThresholdSeconds,
		HeartbeatIntervalSeconds: settings.HeartbeatIntervalSeconds,
		OfflineThresholdSeconds:  settings.OfflineThresholdSeconds,
		UpdatePolicy:             int32(settings.UpdatePolicy),
		LatestVersion:            nullString(settings.LatestVersion),
		UpdateURL:                nullString(settings.UpdateUrl),
		ServerTime:               formatTime(now),
	})
}

func (h *Handler) ClientReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式")
		return
	}

	token := readBearerToken(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "缺少令牌")
		return
	}

	clientToken, err := h.Queries.GetToken(r.Context(), token)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusUnauthorized, "令牌无效")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "令牌校验失败")
		return
	}
	if clientToken.Revoked {
		writeError(w, http.StatusUnauthorized, "令牌已失效")
		return
	}
	if clientToken.ExpiresAt.Valid && clientToken.ExpiresAt.Time.Before(time.Now()) {
		writeError(w, http.StatusUnauthorized, "令牌已过期")
		return
	}

	var payload ClientReportRequest
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "参数格式错误")
		return
	}

	employee, err := h.Queries.GetEmployeeByID(r.Context(), clientToken.EmployeeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "员工不存在")
		return
	}
	if !employee.Enabled {
		writeError(w, http.StatusForbidden, "员工已停用")
		return
	}

	now := time.Now()
	_ = h.Queries.UpdateTokenLastSeen(r.Context(), sqlc.UpdateTokenLastSeenParams{
		LastSeen: sql.NullTime{Time: now, Valid: true},
		Token:    token,
	})

	settings := h.getSettingsOrDefault(r)
	if settings.UpdatePolicy == 1 && settings.LatestVersion.Valid {
		if isVersionOutdated(payload.ClientVersion, settings.LatestVersion.String) {
			writeError(w, http.StatusUpgradeRequired, "请先更新客户端")
			return
		}
	}

	reportType := strings.TrimSpace(payload.ReportType)
	if reportType != "work_end" {
		if err := h.handleWorkSessionReport(r.Context(), employee.ID, reportType, now); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	rules, _ := h.Queries.ListEnabledRules(r.Context())

	status := determineStatus(payload.IdleSeconds, settings.IdleThresholdSeconds, payload.ProcessName, payload.WindowTitle, rules)
	description := buildDescription(payload.ProcessName, payload.WindowTitle)
	if reportType == "break" {
		status = "break"
		description = "休息中"
	}

	prevEvent, prevErr := h.Queries.GetLastRawEventByEmployee(r.Context(), employee.ID)

	if err := h.Queries.CreateRawEvent(r.Context(), sqlc.CreateRawEventParams{
		EmployeeID:    employee.ID,
		ReceivedAt:    now,
		ProcessName:   toNullString(payload.ProcessName),
		WindowTitle:   toNullString(payload.WindowTitle),
		IdleSeconds:   payload.IdleSeconds,
		Status:        sqlc.RawEventsStatus(status),
		ClientVersion: toNullString(payload.ClientVersion),
		IpAddress:     toNullString(clientIP(r)),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "写入上报失败")
		return
	}

	_ = h.Queries.UpdateEmployeeLastSeen(r.Context(), sqlc.UpdateEmployeeLastSeenParams{
		LastSeenAt:      sql.NullTime{Time: now, Valid: true},
		LastStatus:      sqlc.NullEmployeesLastStatus{EmployeesLastStatus: sqlc.EmployeesLastStatus(status), Valid: true},
		LastDescription: toNullString(description),
		ID:              employee.ID,
	})

	if prevErr == nil {
		segmentStart := prevEvent.ReceivedAt
		if employee.LastSegmentEndAt.Valid && employee.LastSegmentEndAt.Time.After(segmentStart) {
			segmentStart = employee.LastSegmentEndAt.Time
		}
		gap := now.Sub(prevEvent.ReceivedAt)
		if now.After(segmentStart) {
			if gap > time.Duration(settings.OfflineThresholdSeconds)*time.Second {
				h.createSegmentAndStatsByContext(r.Context(), employee.ID, segmentStart, now, "offline", "", "offline")
			} else {
				prevDesc := buildDescription(nullString(prevEvent.ProcessName), nullString(prevEvent.WindowTitle))
				h.createSegmentAndStatsByContext(r.Context(), employee.ID, segmentStart, now, string(prevEvent.Status), prevDesc, "system")
			}
		}
	}

	segmentEnd := now
	if employee.LastSegmentEndAt.Valid && employee.LastSegmentEndAt.Time.After(segmentEnd) {
		segmentEnd = employee.LastSegmentEndAt.Time
	}
	_ = h.Queries.UpdateEmployeeLastSegmentEnd(r.Context(), sqlc.UpdateEmployeeLastSegmentEndParams{
		LastSegmentEndAt: sql.NullTime{Time: segmentEnd, Valid: true},
		ID:               employee.ID,
	})

	workEndAccepted := false
	var workEndErr error
	if reportType == "work_end" {
		workEndAccepted, workEndErr = h.handleWorkEndAfterReport(r.Context(), employee, payload, now)
	}

	isWorking := reportType != "work_end" || !workEndAccepted
	statusForLive := status
	descriptionForLive := description
	if !isWorking {
		statusForLive = "offwork"
		descriptionForLive = "已下班"
	}

	h.Hub.Broadcast(LiveMessage{
		Type: "update",
		Item: LiveView{
			EmployeeCode: employee.EmployeeCode,
			Name:         employee.Name,
			Department:   "",
			StatusCode:   statusForLive,
			StatusLabel:  statusLabel(statusForLive),
			Description:  descriptionForLive,
			LastSeen:     formatTime(now),
			DelaySeconds: 0,
			Working:      isWorking,
		},
		Time: formatTime(now),
	})

	if workEndErr != nil {
		if typed, ok := workEndErr.(*workEndError); ok {
			h.writeJSONWithData(w, typed.Status, typed.Message, typed.Code, typed.Data)
		} else {
			writeError(w, http.StatusBadRequest, workEndErr.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, ClientReportResponse{
		IdleThresholdSeconds:     settings.IdleThresholdSeconds,
		HeartbeatIntervalSeconds: settings.HeartbeatIntervalSeconds,
		OfflineThresholdSeconds:  settings.OfflineThresholdSeconds,
		UpdatePolicy:             int32(settings.UpdatePolicy),
		LatestVersion:            nullString(settings.LatestVersion),
		UpdateURL:                nullString(settings.UpdateUrl),
		ServerTime:               formatTime(now),
	})
}

func readBearerToken(r *http.Request) string {
	value := r.Header.Get("Authorization")
	parts := strings.SplitN(value, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func clientIP(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr
	}
	return ip
}

func (h *Handler) handleWorkSessionReport(ctx context.Context, employeeID int64, reportType string, now time.Time) error {
	reportType = strings.TrimSpace(reportType)
	switch reportType {
	case "work_start":
		if _, err := h.Queries.GetOpenWorkSessionByEmployee(ctx, employeeID); err == nil {
			return nil
		} else if err != sql.ErrNoRows {
			return fmt.Errorf("读取上班状态失败")
		}
		if err := h.Queries.CreateWorkSession(ctx, sqlc.CreateWorkSessionParams{
			EmployeeID: employeeID,
			StartAt:    now,
		}); err != nil {
			return fmt.Errorf("写入上班记录失败")
		}
	case "work_end":
		if err := h.Queries.CloseWorkSession(ctx, sqlc.CloseWorkSessionParams{
			EmployeeID: employeeID,
			EndAt:      now,
		}); err != nil {
			return fmt.Errorf("写入下班记录失败")
		}
	}
	return nil
}
func determineStatus(idleSeconds int32, idleThreshold int32, processName string, windowTitle string, rules []sqlc.ListEnabledRulesRow) string {
	if idleSeconds >= idleThreshold {
		return "idle"
	}

	processName = normalizeString(processName)
	windowTitle = normalizeString(windowTitle)

	for _, rule := range rules {
		if rule.RuleType != sqlc.RulesRuleTypeBlack {
			continue
		}
		if ruleMatch(rule, processName, windowTitle) {
			return "fish"
		}
	}

	for _, rule := range rules {
		if rule.RuleType != sqlc.RulesRuleTypeWhite {
			continue
		}
		if ruleMatch(rule, processName, windowTitle) {
			return "work"
		}
	}

	return "normal"
}

func ruleMatch(rule sqlc.ListEnabledRulesRow, processName string, windowTitle string) bool {
	matchValue := normalizeString(rule.MatchValue)
	if matchValue == "" {
		return false
	}
	switch rule.MatchMode {
	case sqlc.RulesMatchModeProcess:
		return processName == matchValue
	case sqlc.RulesMatchModeTitle:
		return strings.Contains(windowTitle, matchValue)
	default:
		return false
	}
}

func (h *Handler) createSegmentAndStatsByContext(ctx context.Context, employeeID int64, start time.Time, end time.Time, status string, description string, source string) {
	if end.Before(start) || end.Equal(start) {
		return
	}

	status = strings.TrimSpace(status)
	source = strings.TrimSpace(source)
	description = strings.TrimSpace(description)

	if h.DB != nil {
		var lastID int64
		var lastEnd time.Time
		var lastStatus string
		var lastSource string
		var lastDesc sql.NullString

		err := h.DB.QueryRowContext(ctx, `SELECT id, end_at, status, description, source
FROM time_segments WHERE employee_id = ? ORDER BY end_at DESC, id DESC LIMIT 1`, employeeID).Scan(&lastID, &lastEnd, &lastStatus, &lastDesc, &lastSource)
		if err == nil {
			lastDescription := strings.TrimSpace(nullString(lastDesc))
			if lastEnd.Equal(start) && lastStatus == status && lastSource == source && lastDescription == description {
				if result, err := h.DB.ExecContext(ctx, "UPDATE time_segments SET end_at = ? WHERE id = ? AND end_at = ?", end, lastID, lastEnd); err == nil {
					if rows, rowsErr := result.RowsAffected(); rowsErr == nil && rows > 0 {
						h.addDailyStatsByRange(ctx, employeeID, status, start, end)
						return
					}
				}
			}
		}
	}

	_ = h.Queries.CreateTimeSegment(ctx, sqlc.CreateTimeSegmentParams{
		EmployeeID:  employeeID,
		StartAt:     start,
		EndAt:       end,
		Status:      sqlc.TimeSegmentsStatus(status),
		Description: toNullString(description),
		Source:      sqlc.TimeSegmentsSource(source),
	})

	h.addDailyStatsByRange(ctx, employeeID, status, start, end)
}

func (h *Handler) addDailyStatsByRange(ctx context.Context, employeeID int64, status string, start time.Time, end time.Time) {
	for _, part := range splitByDay(start, end) {
		increments := buildDailyStatIncrement(status, part.Seconds)
		_ = h.Queries.AddDailyStats(ctx, sqlc.AddDailyStatsParams{
			StatDate:          part.Date,
			EmployeeID:        employeeID,
			WorkSeconds:       increments.Work,
			NormalSeconds:     increments.Normal,
			FishSeconds:       increments.Fish,
			IdleSeconds:       increments.Idle,
			OfflineSeconds:    increments.Offline,
			AttendanceSeconds: increments.Attendance,
			EffectiveSeconds:  increments.Effective,
		})
	}
}

func (h *Handler) createSegmentAndStats(r *http.Request, employeeID int64, start time.Time, end time.Time, status string, description string, source string) {
	h.createSegmentAndStatsByContext(r.Context(), employeeID, start, end, status, description, source)
}

func (h *Handler) createSegmentOnly(r *http.Request, employeeID int64, start time.Time, end time.Time, status string, description string, source string) {
	if end.Before(start) || end.Equal(start) {
		return
	}
	_ = h.Queries.CreateTimeSegment(r.Context(), sqlc.CreateTimeSegmentParams{
		EmployeeID:  employeeID,
		StartAt:     start,
		EndAt:       end,
		Status:      sqlc.TimeSegmentsStatus(status),
		Description: toNullString(description),
		Source:      sqlc.TimeSegmentsSource(source),
	})
}

type dayPart struct {
	Date    time.Time
	Seconds int64
}

func splitByDay(start time.Time, end time.Time) []dayPart {
	if end.Before(start) {
		return nil
	}
	var parts []dayPart
	current := start
	for current.Before(end) {
		nextDay := time.Date(current.Year(), current.Month(), current.Day()+1, 0, 0, 0, 0, current.Location())
		segmentEnd := end
		if nextDay.Before(end) {
			segmentEnd = nextDay
		}
		seconds := int64(segmentEnd.Sub(current).Seconds())
		parts = append(parts, dayPart{
			Date:    time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, current.Location()),
			Seconds: seconds,
		})
		current = segmentEnd
	}
	return parts
}

type dailyIncrement struct {
	Work       int32
	Normal     int32
	Fish       int32
	Idle       int32
	Offline    int32
	Attendance int32
	Effective  int32
}

func buildDailyStatIncrement(status string, seconds int64) dailyIncrement {
	inc := dailyIncrement{}
	sec := int32(seconds)
	switch status {
	case "work":
		inc.Work = sec
		inc.Attendance = sec
		inc.Effective = sec
	case "normal":
		inc.Normal = sec
		inc.Attendance = sec
	case "fish":
		inc.Fish = sec
		inc.Attendance = sec
	case "idle":
		inc.Idle = sec
	case "offline":
		inc.Offline = sec
	case "incident":
		inc.Offline = 0
	}
	return inc
}

func isVersionOutdated(clientVersion string, latestVersion string) bool {
	clientVersion = strings.TrimSpace(clientVersion)
	latestVersion = strings.TrimSpace(latestVersion)
	if latestVersion == "" {
		return false
	}
	if clientVersion == "" {
		return true
	}
	return compareVersion(clientVersion, latestVersion) < 0
}

func compareVersion(a string, b string) int {
	aParts := parseVersionParts(a)
	bParts := parseVersionParts(b)
	maxLen := len(aParts)
	if len(bParts) > maxLen {
		maxLen = len(bParts)
	}
	for i := 0; i < maxLen; i++ {
		av := 0
		bv := 0
		if i < len(aParts) {
			av = aParts[i]
		}
		if i < len(bParts) {
			bv = bParts[i]
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}

func parseVersionParts(version string) []int {
	parts := strings.FieldsFunc(version, func(r rune) bool {
		return strings.ContainsRune(".-_+", r)
	})
	values := make([]int, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			value = 0
		}
		values = append(values, value)
	}
	return values
}
