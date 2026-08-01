package handlers

import (
	"database/sql"
	"time"

	"worksentry/internal/db/sqlc"
)

type dailyReportMetrics struct {
	WorkSeconds   int64
	IdleSeconds   int64
	OnDutySeconds int64
}

func buildDailyReportMetrics(normalSeconds int64, fishSeconds int64, idleSeconds int64, offlineSeconds int64, effectiveSeconds int64, breakSeconds int64, onDutySeconds int64) dailyReportMetrics {
	mergedIdleSeconds := idleSeconds + breakSeconds
	if mergedIdleSeconds < 0 {
		mergedIdleSeconds = 0
	}

	workSeconds := effectiveSeconds + normalSeconds + fishSeconds + mergedIdleSeconds + offlineSeconds
	if workSeconds < 0 {
		workSeconds = 0
	}
	if onDutySeconds < 0 {
		onDutySeconds = 0
	}

	return dailyReportMetrics{
		WorkSeconds:   workSeconds,
		IdleSeconds:   mergedIdleSeconds,
		OnDutySeconds: onDutySeconds,
	}
}

func buildDailyStatsByDateParams(date time.Time, departmentID int64) sqlc.ListDailyStatsByDateParams {
	dayStart, _, reportEnd := reportDayBounds(date)

	return sqlc.ListDailyStatsByDateParams{
		DayStart:           dayStart,
		ReportEnd:          reportEnd,
		StatDate:           date,
		DepartmentIDFilter: departmentID,
		DepartmentID:       toNullInt64(departmentID),
	}
}

func buildDailyStatsExportParams(date time.Time, departmentID int64) sqlc.ListDailyStatsForExportByDateParams {
	dayStart, dayEnd, reportEnd := reportDayBounds(date)
	reportEndNull := sql.NullTime{Time: reportEnd, Valid: true}
	dayStartNull := sql.NullTime{Time: dayStart, Valid: true}
	dayEndNull := sql.NullTime{Time: dayEnd, Valid: true}

	return sqlc.ListDailyStatsForExportByDateParams{
		GREATEST:     dayStart,
		LEAST:        reportEnd,
		StartAt:      reportEnd,
		EndAt:        dayStart,
		GREATEST_2:   dayStart,
		Column6:      reportEnd,
		LEAST_2:      reportEnd,
		StartAt_2:    reportEnd,
		EndAt_2:      reportEndNull,
		EndAt_3:      dayStartNull,
		StartAt_3:    dayStart,
		StartAt_4:    dayEnd,
		EndAt_4:      dayStartNull,
		EndAt_5:      dayEndNull,
		StatDate:     date,
		Column16:     departmentID,
		DepartmentID: toNullInt64(departmentID),
	}
}

func reportDayBounds(date time.Time) (time.Time, time.Time, time.Time) {
	dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.Local)
	dayEnd := dayStart.Add(24 * time.Hour)
	reportEnd := dayEnd

	now := time.Now()
	if isSameLocalDate(dayStart, now) && now.Before(dayEnd) {
		reportEnd = now
	}

	return dayStart, dayEnd, reportEnd
}

func isSameLocalDate(a time.Time, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

func interfaceToNullTime(value any) sql.NullTime {
	switch v := value.(type) {
	case nil:
		return sql.NullTime{}
	case sql.NullTime:
		return v
	case time.Time:
		return sql.NullTime{Time: v, Valid: true}
	case []byte:
		if t, err := parseDateTime(string(v)); err == nil {
			return sql.NullTime{Time: t, Valid: true}
		}
	case string:
		if t, err := parseDateTime(v); err == nil {
			return sql.NullTime{Time: t, Valid: true}
		}
	}

	return sql.NullTime{}
}
