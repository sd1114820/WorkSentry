package handlers

import (
	"database/sql"
	"testing"

	"worksentry/internal/db/sqlc"
)

func TestAuditTargetIncludesTargetID(t *testing.T) {
	item := sqlc.AuditLog{TargetType: "employee", TargetID: sql.NullInt64{Int64: 42, Valid: true}}
	if got := auditTarget(item); got != "employee #42" {
		t.Fatalf("审计目标 = %q，期望 %q", got, "employee #42")
	}
}
