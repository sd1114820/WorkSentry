package http

import (
	"net/http"

	"worksentry/internal/http/handlers"
)

func NewRouter(h *handlers.Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", h.Health)

	// 客户端接口
	mux.HandleFunc("/api/v1/client/bind", h.ClientBind)
	mux.HandleFunc("/api/v1/client/report", h.ClientReport)
	mux.HandleFunc("/api/v1/client/checkout-template", h.ClientCheckoutTemplate)

	adminOnly := func(fn http.HandlerFunc) http.HandlerFunc {
		return h.AdminOnly(fn)
	}

	// 管理端接口
	mux.HandleFunc("/api/v1/admin/login", h.AdminLogin)
	mux.HandleFunc("/api/v1/admin/admin-users", adminOnly(h.AdminUsers))
	mux.HandleFunc("/api/v1/admin/password", adminOnly(h.AdminChangePassword))
	mux.HandleFunc("/api/v1/admin/settings", adminOnly(h.Settings))
	mux.HandleFunc("/api/v1/admin/rules", adminOnly(h.Rules))
	mux.HandleFunc("/api/v1/admin/live-snapshot", adminOnly(h.LiveSnapshot))
	mux.HandleFunc("/api/v1/admin/reports/daily", adminOnly(h.ReportDaily))
	mux.HandleFunc("/api/v1/admin/reports/timeline", adminOnly(h.ReportTimeline))
	mux.HandleFunc("/api/v1/admin/reports/rank", adminOnly(h.ReportRank))
	mux.HandleFunc("/api/v1/admin/department-rules", adminOnly(h.DepartmentRules))
	mux.HandleFunc("/api/v1/admin/work-session-reviews", adminOnly(h.WorkSessionReviews))
	mux.HandleFunc("/api/v1/admin/work-session-review", adminOnly(h.WorkSessionReviewDetail))
	mux.HandleFunc("/api/v1/admin/exports/daily.xlsx", adminOnly(h.ExportDaily))
	mux.HandleFunc("/api/v1/admin/manual-adjustments", adminOnly(h.ManualAdjustments))
	mux.HandleFunc("/api/v1/admin/offline-segments", adminOnly(h.OfflineSegments))
	mux.HandleFunc("/api/v1/admin/system-incidents", adminOnly(h.SystemIncidents))
	mux.HandleFunc("/api/v1/admin/audit-logs", adminOnly(h.AuditLogs))
	mux.HandleFunc("/api/v1/admin/departments", adminOnly(h.Departments))
	mux.HandleFunc("/api/v1/admin/employees", adminOnly(h.Employees))
	mux.HandleFunc("/api/v1/admin/employees/unbind", adminOnly(h.UnbindEmployee))
	mux.HandleFunc("/api/v1/admin/checkout-templates", adminOnly(h.CheckoutTemplates))
	mux.HandleFunc("/api/v1/admin/checkout-fields", adminOnly(h.CheckoutFields))
	mux.HandleFunc("/api/v1/admin/checkout-records", adminOnly(h.CheckoutRecords))
	mux.HandleFunc("/api/v1/admin/checkout-record", adminOnly(h.CheckoutRecordDetail))

	// WebSocket
	mux.HandleFunc("/ws/v1/live", h.LiveWS)

	mux.Handle("/", h.Static())

	return h.WithLogging(mux)
}
