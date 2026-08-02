package db

import (
	"context"
	"testing"
)

func TestHistoryCleanupCompatibilityMigrationDoesNotAccessDatabase(t *testing.T) {
	if err := migrate008HistoryCleanupCompatibility(context.Background(), nil); err != nil {
		t.Fatalf("历史数据清理兼容迁移不应执行数据库操作: %v", err)
	}
}

func TestHistoryCleanupCompatibilityMigrationVersionIsPreserved(t *testing.T) {
	found := false
	for _, migration := range migrations {
		if migration.Version == 8 {
			found = true
			if migration.Name != "history_cleanup_indexes" {
				t.Fatalf("第八版兼容迁移名称发生变化: %q", migration.Name)
			}
		}
	}
	if !found {
		t.Fatal("缺少第八版兼容迁移")
	}
}

func TestAuditLogQueryIndexMigrationIsRegistered(t *testing.T) {
	found := false
	for _, migration := range migrations {
		if migration.Version == 10 {
			found = true
			if migration.Name != "audit_log_query_index" {
				t.Fatalf("第十版迁移名称错误: %q", migration.Name)
			}
		}
	}
	if !found {
		t.Fatal("缺少审计日志索引升级迁移")
	}
}
