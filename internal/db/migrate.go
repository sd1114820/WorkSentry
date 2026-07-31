package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"
)

type Migration struct {
	Version int64
	Name    string
	Run     func(context.Context, *sql.DB) error
}

var migrations = []Migration{
	{Version: 1, Name: "init", Run: migrate001Init},
	{Version: 2, Name: "work_sessions", Run: migrate002WorkSessions},
	{Version: 3, Name: "checkout_templates", Run: migrate003CheckoutTemplates},
	{Version: 4, Name: "department_rules", Run: migrate004DepartmentRules},
	{Version: 5, Name: "perf_indexes", Run: migrate005PerfIndexes},
	{Version: 6, Name: "storage_growth_optimization", Run: migrate006StorageGrowthOptimization},
	{Version: 7, Name: "ingest_outbox", Run: migrate007IngestOutbox},
}

func Migrate(ctx context.Context, db *sql.DB) error {
	log.Printf("开始检查数据库迁移")

	if err := ensureSchemaMigrationsTable(ctx, db); err != nil {
		return err
	}

	applied, err := loadAppliedMigrationVersions(ctx, db)
	if err != nil {
		return err
	}

	if len(applied) == 0 {
		adopted, err := adoptLegacyMigrations(ctx, db)
		if err != nil {
			return err
		}
		if len(adopted) > 0 {
			names := make([]string, 0, len(adopted))
			for _, migration := range adopted {
				names = append(names, fmt.Sprintf("%03d %s", migration.Version, migration.Name))
			}
			log.Printf("检测到旧库结构，已认领历史迁移: %s", strings.Join(names, ", "))
			applied, err = loadAppliedMigrationVersions(ctx, db)
			if err != nil {
				return err
			}
		}
	}

	executed := 0
	for _, migration := range migrations {
		if applied[migration.Version] {
			continue
		}

		start := time.Now()
		log.Printf("执行数据库迁移 %03d %s", migration.Version, migration.Name)
		if err := migration.Run(ctx, db); err != nil {
			return fmt.Errorf("迁移 %03d %s 失败: %w", migration.Version, migration.Name, err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO schema_migrations (version, name) VALUES (?, ?)`, migration.Version, migration.Name); err != nil {
			return fmt.Errorf("记录迁移 %03d %s 失败: %w", migration.Version, migration.Name, err)
		}
		log.Printf("数据库迁移 %03d %s 完成，耗时 %s", migration.Version, migration.Name, time.Since(start).Round(time.Millisecond))
		executed++
	}

	if executed == 0 {
		log.Printf("数据库迁移检查完成，无需执行")
	} else {
		log.Printf("数据库迁移全部完成，共执行 %d 个版本", executed)
	}
	return nil
}

func ensureSchemaMigrationsTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
  version BIGINT PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err != nil {
		return fmt.Errorf("创建迁移记录表失败: %w", err)
	}
	return nil
}

func loadAppliedMigrationVersions(ctx context.Context, db *sql.DB) (map[int64]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("读取迁移记录失败: %w", err)
	}
	defer rows.Close()

	versions := make(map[int64]bool, len(migrations))
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("读取迁移版本失败: %w", err)
		}
		versions[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取迁移记录失败: %w", err)
	}
	return versions, nil
}

func adoptLegacyMigrations(ctx context.Context, db *sql.DB) ([]Migration, error) {
	adopted := make([]Migration, 0, len(migrations))
	for _, migration := range migrations {
		present, err := isMigrationAlreadyPresent(ctx, db, migration.Version)
		if err != nil {
			return nil, err
		}
		if !present {
			break
		}
		if _, err := db.ExecContext(ctx, `INSERT IGNORE INTO schema_migrations (version, name) VALUES (?, ?)`, migration.Version, migration.Name); err != nil {
			return nil, fmt.Errorf("认领迁移 %03d %s 失败: %w", migration.Version, migration.Name, err)
		}
		adopted = append(adopted, migration)
	}
	return adopted, nil
}

func isMigrationAlreadyPresent(ctx context.Context, db *sql.DB, version int64) (bool, error) {
	switch version {
	case 1:
		return hasInitSchema(ctx, db)
	case 2:
		return tableExists(ctx, db, "work_sessions")
	case 3:
		return allTablesExist(ctx, db, "checkout_templates", "checkout_fields", "work_session_checkouts")
	case 4:
		if !allTablesExistOrFalse(ctx, db, "department_work_rules", "department_status_thresholds", "work_session_reviews") {
			return false, nil
		}
		return allEnumValuesPresent(ctx, db,
			enumCheck{Table: "employees", Column: "last_status", Value: "break"},
			enumCheck{Table: "raw_events", Column: "status", Value: "break"},
			enumCheck{Table: "time_segments", Column: "status", Value: "break"},
		)
	case 5:
		return allIndexesExist(ctx, db,
			indexCheck{Table: "time_segments", Index: "idx_time_segments_employee_end"},
			indexCheck{Table: "work_sessions", Index: "idx_work_sessions_end_employee_start"},
			indexCheck{Table: "employees", Index: "idx_employees_enabled_last_seen"},
		)
	case 6:
		columnReady, err := columnExists(ctx, db, "employees", "current_segment_started_at")
		if err != nil {
			return false, err
		}
		if !columnReady {
			return false, nil
		}
		indexReady, err := indexExists(ctx, db, "raw_events", "idx_raw_events_received_at")
		if err != nil {
			return false, err
		}
		if !indexReady {
			return false, nil
		}
		return allEnumValuesPresent(ctx, db,
			enumCheck{Table: "employees", Column: "last_status", Value: "offline"},
			enumCheck{Table: "raw_events", Column: "status", Value: "offline"},
		)
	case 7:
		tablesReady, err := allTablesExist(ctx, db,
			"client_report_outbox",
			"processed_ingests",
			"processed_source_events",
			"stat_deltas",
		)
		if err != nil || !tablesReady {
			return tablesReady, err
		}
		for _, column := range []string{"ingest_id", "source_event_id", "client_event_id"} {
			columnReady, err := columnExists(ctx, db, "raw_events", column)
			if err != nil || !columnReady {
				return columnReady, err
			}
		}
		return allIndexesExist(ctx, db,
			indexCheck{Table: "client_report_outbox", Index: "idx_client_report_outbox_status_created"},
			indexCheck{Table: "client_report_outbox", Index: "idx_client_report_outbox_employee_time"},
			indexCheck{Table: "client_report_outbox", Index: "idx_client_report_outbox_source_event"},
			indexCheck{Table: "processed_source_events", Index: "idx_processed_source_events_employee"},
			indexCheck{Table: "stat_deltas", Index: "idx_stat_deltas_date_employee"},
			indexCheck{Table: "stat_deltas", Index: "idx_stat_deltas_ingest"},
			indexCheck{Table: "stat_deltas", Index: "idx_stat_deltas_source_event"},
			indexCheck{Table: "raw_events", Index: "idx_raw_events_ingest"},
			indexCheck{Table: "raw_events", Index: "idx_raw_events_source_event"},
		)
	default:
		return false, nil
	}
}

func hasInitSchema(ctx context.Context, db *sql.DB) (bool, error) {
	tablesReady, err := allTablesExist(ctx, db,
		"departments",
		"employees",
		"admin_users",
		"admin_sessions",
		"client_tokens",
		"rules",
		"settings",
		"raw_events",
		"time_segments",
		"daily_stats",
		"manual_adjustments",
		"system_incidents",
		"audit_logs",
	)
	if err != nil || !tablesReady {
		return tablesReady, err
	}
	return ruleExists(ctx, db, sql.NullString{String: "black", Valid: true}, sql.NullString{String: "process", Valid: true}, "steam.exe")
}

func migrate001Init(ctx context.Context, db *sql.DB) error {
	return execStatements(ctx, db,
		`CREATE TABLE IF NOT EXISTS departments (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  name VARCHAR(64) NOT NULL,
  parent_id BIGINT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS employees (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  employee_code VARCHAR(32) NOT NULL UNIQUE,
  name VARCHAR(64) NOT NULL,
  department_id BIGINT NULL,
  fingerprint_hash VARCHAR(128) NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  last_seen_at DATETIME NULL,
  last_status ENUM('work','normal','fish','idle') NULL,
  last_description VARCHAR(255) NULL,
  last_segment_end_at DATETIME NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_employees_department (department_id),
  INDEX idx_employees_last_seen (last_seen_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS admin_users (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  username VARCHAR(64) NOT NULL UNIQUE,
  password_hash VARCHAR(255) NOT NULL,
  display_name VARCHAR(64) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS admin_sessions (
  token VARCHAR(64) PRIMARY KEY,
  admin_id BIGINT NOT NULL,
  issued_at DATETIME NOT NULL,
  expires_at DATETIME NULL,
  revoked TINYINT(1) NOT NULL DEFAULT 0,
  last_seen DATETIME NULL,
  INDEX idx_admin_sessions_admin (admin_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS client_tokens (
  token VARCHAR(64) PRIMARY KEY,
  employee_id BIGINT NOT NULL,
  issued_at DATETIME NOT NULL,
  expires_at DATETIME NULL,
  revoked TINYINT(1) NOT NULL DEFAULT 0,
  last_seen DATETIME NULL,
  INDEX idx_client_tokens_employee (employee_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS rules (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  rule_type ENUM('white','black') NOT NULL,
  match_mode ENUM('process','title') NOT NULL,
  match_value VARCHAR(255) NOT NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  remark VARCHAR(255) NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS settings (
  id TINYINT PRIMARY KEY,
  idle_threshold_seconds INT NOT NULL DEFAULT 300,
  heartbeat_interval_seconds INT NOT NULL DEFAULT 300,
  offline_threshold_seconds INT NOT NULL DEFAULT 600,
  fish_ratio_warn_percent INT NOT NULL DEFAULT 10,
  update_policy TINYINT NOT NULL DEFAULT 0,
  latest_version VARCHAR(32) NULL,
  update_url VARCHAR(255) NULL,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS raw_events (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  employee_id BIGINT NOT NULL,
  received_at DATETIME NOT NULL,
  process_name VARCHAR(255) NULL,
  window_title VARCHAR(512) NULL,
  idle_seconds INT NOT NULL,
  status ENUM('work','normal','fish','idle') NOT NULL,
  client_version VARCHAR(32) NULL,
  ip_address VARCHAR(64) NULL,
  INDEX idx_raw_events_employee_time (employee_id, received_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS time_segments (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  employee_id BIGINT NOT NULL,
  start_at DATETIME NOT NULL,
  end_at DATETIME NOT NULL,
  status ENUM('work','normal','fish','idle','offline','incident') NOT NULL,
  description VARCHAR(255) NULL,
  source ENUM('system','offline','manual','incident') NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_time_segments_employee_time (employee_id, start_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS daily_stats (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  stat_date DATE NOT NULL,
  employee_id BIGINT NOT NULL,
  work_seconds INT NOT NULL DEFAULT 0,
  normal_seconds INT NOT NULL DEFAULT 0,
  fish_seconds INT NOT NULL DEFAULT 0,
  idle_seconds INT NOT NULL DEFAULT 0,
  offline_seconds INT NOT NULL DEFAULT 0,
  attendance_seconds INT NOT NULL DEFAULT 0,
  effective_seconds INT NOT NULL DEFAULT 0,
  UNIQUE KEY uk_daily_stats_date_employee (stat_date, employee_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS manual_adjustments (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  employee_id BIGINT NOT NULL,
  start_at DATETIME NOT NULL,
  end_at DATETIME NOT NULL,
  reason VARCHAR(64) NOT NULL,
  note TEXT NOT NULL,
  operator_id BIGINT NOT NULL,
  status ENUM('active','revoked') NOT NULL DEFAULT 'active',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_manual_adjustments_employee_time (employee_id, start_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS system_incidents (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  start_at DATETIME NOT NULL,
  end_at DATETIME NOT NULL,
  reason VARCHAR(64) NOT NULL,
  note TEXT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  operator_id BIGINT NOT NULL,
  action VARCHAR(64) NOT NULL,
  target_type VARCHAR(64) NOT NULL,
  target_id BIGINT NULL,
  detail JSON NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		builtinBlacklistInsertSQL,
	)
}

func migrate002WorkSessions(ctx context.Context, db *sql.DB) error {
	return execStatements(ctx, db,
		`CREATE TABLE IF NOT EXISTS work_sessions (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  employee_id BIGINT NOT NULL,
  start_at DATETIME NOT NULL,
  end_at DATETIME NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_work_sessions_employee_start (employee_id, start_at),
  INDEX idx_work_sessions_employee_end (employee_id, end_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	)
}

func migrate003CheckoutTemplates(ctx context.Context, db *sql.DB) error {
	return execStatements(ctx, db,
		`CREATE TABLE IF NOT EXISTS checkout_templates (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  department_id BIGINT NOT NULL,
  name_zh VARCHAR(100) NOT NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_checkout_templates_department (department_id),
  INDEX idx_checkout_templates_enabled (enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS checkout_fields (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  template_id BIGINT NOT NULL,
  name_zh VARCHAR(100) NOT NULL,
  type ENUM('text','number','select') NOT NULL,
  required TINYINT(1) NOT NULL DEFAULT 0,
  sort_order INT NOT NULL DEFAULT 0,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  options_zh_json TEXT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_checkout_fields_template (template_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS work_session_checkouts (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  work_session_id BIGINT NOT NULL,
  template_id BIGINT NOT NULL,
  template_snapshot_json TEXT NOT NULL,
  data_json TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_work_session_checkouts_session (work_session_id),
  INDEX idx_work_session_checkouts_template (template_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	)
}

func migrate004DepartmentRules(ctx context.Context, db *sql.DB) error {
	if err := ensureEnumContains(ctx, db, "employees", "last_status", "break", `ALTER TABLE employees MODIFY last_status ENUM('work','normal','fish','idle','break') NULL`); err != nil {
		return err
	}
	if err := ensureEnumContains(ctx, db, "raw_events", "status", "break", `ALTER TABLE raw_events MODIFY status ENUM('work','normal','fish','idle','break') NOT NULL`); err != nil {
		return err
	}
	if err := ensureEnumContains(ctx, db, "time_segments", "status", "break", `ALTER TABLE time_segments MODIFY status ENUM('work','normal','fish','idle','offline','incident','break') NOT NULL`); err != nil {
		return err
	}
	return execStatements(ctx, db,
		`CREATE TABLE IF NOT EXISTS department_work_rules (
  department_id BIGINT PRIMARY KEY,
  target_seconds INT NOT NULL DEFAULT 0,
  max_break_seconds INT NOT NULL DEFAULT 0,
  max_break_count INT NOT NULL DEFAULT 0,
  max_break_single_seconds INT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS department_status_thresholds (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  department_id BIGINT NOT NULL,
  status_code VARCHAR(32) NOT NULL,
  min_seconds INT NULL,
  max_seconds INT NULL,
  trigger_action ENUM('show_only','require_reason') NOT NULL DEFAULT 'show_only',
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_department_status_threshold (department_id, status_code),
  INDEX idx_department_status_threshold_department (department_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS work_session_reviews (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  work_session_id BIGINT NOT NULL,
  employee_id BIGINT NOT NULL,
  department_id BIGINT NULL,
  work_date DATE NOT NULL,
  work_standard_seconds INT NOT NULL DEFAULT 0,
  break_seconds INT NOT NULL DEFAULT 0,
  need_reason TINYINT(1) NOT NULL DEFAULT 0,
  reason TEXT NULL,
  violations_json TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_work_session_reviews_session (work_session_id),
  INDEX idx_work_session_reviews_date (work_date),
  INDEX idx_work_session_reviews_employee (employee_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	)
}

func migrate005PerfIndexes(ctx context.Context, db *sql.DB) error {
	if err := ensureIndex(ctx, db, "time_segments", "idx_time_segments_employee_end", `ALTER TABLE time_segments ADD INDEX idx_time_segments_employee_end (employee_id, end_at, id)`); err != nil {
		return err
	}
	if err := ensureIndex(ctx, db, "work_sessions", "idx_work_sessions_end_employee_start", `ALTER TABLE work_sessions ADD INDEX idx_work_sessions_end_employee_start (end_at, employee_id, start_at)`); err != nil {
		return err
	}
	if err := ensureIndex(ctx, db, "employees", "idx_employees_enabled_last_seen", `ALTER TABLE employees ADD INDEX idx_employees_enabled_last_seen (enabled, last_seen_at)`); err != nil {
		return err
	}
	return nil
}

func migrate006StorageGrowthOptimization(ctx context.Context, db *sql.DB) error {
	if err := ensureEnumContains(ctx, db, "employees", "last_status", "offline", `ALTER TABLE employees MODIFY last_status ENUM('work','normal','fish','idle','break','offline') NULL`); err != nil {
		return err
	}
	if err := ensureColumn(ctx, db, "employees", "current_segment_started_at", `ALTER TABLE employees ADD COLUMN current_segment_started_at DATETIME NULL AFTER last_description`); err != nil {
		return err
	}
	if err := ensureEnumContains(ctx, db, "raw_events", "status", "offline", `ALTER TABLE raw_events MODIFY status ENUM('work','normal','fish','idle','break','offline') NOT NULL`); err != nil {
		return err
	}
	if err := ensureIndex(ctx, db, "raw_events", "idx_raw_events_received_at", `ALTER TABLE raw_events ADD INDEX idx_raw_events_received_at (received_at, id)`); err != nil {
		return err
	}
	return nil
}

func migrate007IngestOutbox(ctx context.Context, db *sql.DB) error {
	if err := execStatements(ctx, db,
		`CREATE TABLE IF NOT EXISTS client_report_outbox (
  ingest_id CHAR(36) PRIMARY KEY,
  source_event_id VARCHAR(128) NULL,
  client_event_id VARCHAR(128) NULL,
  employee_id BIGINT NOT NULL,
  received_at DATETIME NOT NULL,
  payload_json JSON NOT NULL,
  mq_status ENUM('pending','published','failed') NOT NULL DEFAULT 'pending',
  mq_attempts INT NOT NULL DEFAULT 0,
  last_error TEXT NULL,
  published_at DATETIME NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_client_report_outbox_status_created (mq_status, created_at),
  INDEX idx_client_report_outbox_employee_time (employee_id, received_at),
  INDEX idx_client_report_outbox_source_event (source_event_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS processed_ingests (
  ingest_id CHAR(36) PRIMARY KEY,
  processed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS processed_source_events (
  source_event_id VARCHAR(128) PRIMARY KEY,
  first_ingest_id CHAR(36) NOT NULL,
  employee_id BIGINT NOT NULL,
  client_event_id VARCHAR(128) NOT NULL,
  processed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_processed_source_events_employee (employee_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS stat_deltas (
  event_key VARCHAR(128) PRIMARY KEY,
  ingest_id CHAR(36) NOT NULL,
  source_event_id VARCHAR(128) NULL,
  stat_date DATE NOT NULL,
  employee_id BIGINT NOT NULL,
  work_seconds INT NOT NULL DEFAULT 0,
  normal_seconds INT NOT NULL DEFAULT 0,
  fish_seconds INT NOT NULL DEFAULT 0,
  idle_seconds INT NOT NULL DEFAULT 0,
  offline_seconds INT NOT NULL DEFAULT 0,
  attendance_seconds INT NOT NULL DEFAULT 0,
  effective_seconds INT NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_stat_deltas_date_employee (stat_date, employee_id),
  INDEX idx_stat_deltas_ingest (ingest_id),
  INDEX idx_stat_deltas_source_event (source_event_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	); err != nil {
		return err
	}
	if err := ensureColumn(ctx, db, "raw_events", "ingest_id", `ALTER TABLE raw_events ADD COLUMN ingest_id CHAR(36) NULL AFTER id`); err != nil {
		return err
	}
	if err := ensureColumn(ctx, db, "raw_events", "source_event_id", `ALTER TABLE raw_events ADD COLUMN source_event_id VARCHAR(128) NULL AFTER ingest_id`); err != nil {
		return err
	}
	if err := ensureColumn(ctx, db, "raw_events", "client_event_id", `ALTER TABLE raw_events ADD COLUMN client_event_id VARCHAR(128) NULL AFTER source_event_id`); err != nil {
		return err
	}
	if err := ensureIndex(ctx, db, "raw_events", "idx_raw_events_ingest", `ALTER TABLE raw_events ADD INDEX idx_raw_events_ingest (ingest_id)`); err != nil {
		return err
	}
	if err := ensureIndex(ctx, db, "raw_events", "idx_raw_events_source_event", `ALTER TABLE raw_events ADD INDEX idx_raw_events_source_event (source_event_id)`); err != nil {
		return err
	}
	indexes := []struct {
		table     string
		index     string
		statement string
	}{
		{"client_report_outbox", "idx_client_report_outbox_status_created", `ALTER TABLE client_report_outbox ADD INDEX idx_client_report_outbox_status_created (mq_status, created_at)`},
		{"client_report_outbox", "idx_client_report_outbox_employee_time", `ALTER TABLE client_report_outbox ADD INDEX idx_client_report_outbox_employee_time (employee_id, received_at)`},
		{"client_report_outbox", "idx_client_report_outbox_source_event", `ALTER TABLE client_report_outbox ADD INDEX idx_client_report_outbox_source_event (source_event_id)`},
		{"processed_source_events", "idx_processed_source_events_employee", `ALTER TABLE processed_source_events ADD INDEX idx_processed_source_events_employee (employee_id)`},
		{"stat_deltas", "idx_stat_deltas_date_employee", `ALTER TABLE stat_deltas ADD INDEX idx_stat_deltas_date_employee (stat_date, employee_id)`},
		{"stat_deltas", "idx_stat_deltas_ingest", `ALTER TABLE stat_deltas ADD INDEX idx_stat_deltas_ingest (ingest_id)`},
		{"stat_deltas", "idx_stat_deltas_source_event", `ALTER TABLE stat_deltas ADD INDEX idx_stat_deltas_source_event (source_event_id)`},
	}
	for _, index := range indexes {
		if err := ensureIndex(ctx, db, index.table, index.index, index.statement); err != nil {
			return err
		}
	}
	return nil
}

func execStatements(ctx context.Context, db *sql.DB, statements ...string) error {
	for _, statement := range statements {
		trimmed := strings.TrimSpace(statement)
		if trimmed == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, trimmed); err != nil {
			return err
		}
	}
	return nil
}

func ensureColumn(ctx context.Context, db *sql.DB, table string, column string, statement string) error {
	exists, err := columnExists(ctx, db, table, column)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = db.ExecContext(ctx, statement)
	return err
}

func ensureIndex(ctx context.Context, db *sql.DB, table string, index string, statement string) error {
	exists, err := indexExists(ctx, db, table, index)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = db.ExecContext(ctx, statement)
	return err
}

func ensureEnumContains(ctx context.Context, db *sql.DB, table string, column string, value string, statement string) error {
	contains, err := enumContains(ctx, db, table, column, value)
	if err != nil {
		return err
	}
	if contains {
		return nil
	}
	_, err = db.ExecContext(ctx, statement)
	return err
}

func tableExists(ctx context.Context, db *sql.DB, table string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(1)
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = ?`, table).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("检查表 %s 失败: %w", table, err)
	}
	return count > 0, nil
}

func allTablesExist(ctx context.Context, db *sql.DB, tables ...string) (bool, error) {
	for _, table := range tables {
		exists, err := tableExists(ctx, db, table)
		if err != nil {
			return false, err
		}
		if !exists {
			return false, nil
		}
	}
	return true, nil
}

func allTablesExistOrFalse(ctx context.Context, db *sql.DB, tables ...string) bool {
	ok, err := allTablesExist(ctx, db, tables...)
	if err != nil {
		return false
	}
	return ok
}

type indexCheck struct {
	Table string
	Index string
}

func allIndexesExist(ctx context.Context, db *sql.DB, checks ...indexCheck) (bool, error) {
	for _, check := range checks {
		exists, err := indexExists(ctx, db, check.Table, check.Index)
		if err != nil {
			return false, err
		}
		if !exists {
			return false, nil
		}
	}
	return true, nil
}

type enumCheck struct {
	Table  string
	Column string
	Value  string
}

func allEnumValuesPresent(ctx context.Context, db *sql.DB, checks ...enumCheck) (bool, error) {
	for _, check := range checks {
		contains, err := enumContains(ctx, db, check.Table, check.Column, check.Value)
		if err != nil {
			return false, err
		}
		if !contains {
			return false, nil
		}
	}
	return true, nil
}

func columnExists(ctx context.Context, db *sql.DB, table string, column string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(1)
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = ?
  AND COLUMN_NAME = ?`, table, column).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("检查字段 %s.%s 失败: %w", table, column, err)
	}
	return count > 0, nil
}

func indexExists(ctx context.Context, db *sql.DB, table string, index string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(1)
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = ?
  AND INDEX_NAME = ?`, table, index).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("检查索引 %s.%s 失败: %w", table, index, err)
	}
	return count > 0, nil
}

func enumContains(ctx context.Context, db *sql.DB, table string, column string, value string) (bool, error) {
	var columnType sql.NullString
	err := db.QueryRowContext(ctx, `SELECT COLUMN_TYPE
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = ?
  AND COLUMN_NAME = ?`, table, column).Scan(&columnType)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("检查枚举 %s.%s 失败: %w", table, column, err)
	}
	if !columnType.Valid {
		return false, nil
	}
	needle := "'" + strings.ToLower(strings.TrimSpace(value)) + "'"
	return strings.Contains(strings.ToLower(columnType.String), needle), nil
}

func ruleExists(ctx context.Context, db *sql.DB, ruleType sql.NullString, matchMode sql.NullString, matchValue string) (bool, error) {
	if strings.TrimSpace(matchValue) == "" {
		return false, nil
	}
	var count int
	query := `SELECT COUNT(1) FROM rules WHERE match_value = ?`
	args := []any{matchValue}
	if ruleType.Valid {
		query += ` AND rule_type = ?`
		args = append(args, ruleType.String)
	}
	if matchMode.Valid {
		query += ` AND match_mode = ?`
		args = append(args, matchMode.String)
	}
	if err := db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return false, fmt.Errorf("检查内置规则失败: %w", err)
	}
	return count > 0, nil
}

const builtinBlacklistInsertSQL = `INSERT INTO rules (rule_type, match_mode, match_value, enabled, remark)
SELECT t.rule_type, t.match_mode, t.match_value, 1, t.remark
FROM (
  SELECT 'black' AS rule_type, 'process' AS match_mode, 'steam.exe' AS match_value, '内置：游戏平台/游戏进程' AS remark
  UNION ALL SELECT 'black','process','epicgameslauncher.exe','内置：游戏平台/游戏进程'
  UNION ALL SELECT 'black','process','battle.net.exe','内置：游戏平台/游戏进程'
  UNION ALL SELECT 'black','process','wegame.exe','内置：游戏平台/游戏进程'
  UNION ALL SELECT 'black','process','riotclientservices.exe','内置：游戏平台/游戏进程'
  UNION ALL SELECT 'black','process','leagueclient.exe','内置：游戏平台/游戏进程'
  UNION ALL SELECT 'black','process','valorant.exe','内置：游戏平台/游戏进程'
  UNION ALL SELECT 'black','process','genshinimpact.exe','内置：游戏平台/游戏进程'
  UNION ALL SELECT 'black','process','starrail.exe','内置：游戏平台/游戏进程'
  UNION ALL SELECT 'black','title','bilibili','内置：视频/直播/短视频'
  UNION ALL SELECT 'black','title','哔哩哔哩','内置：视频/直播/短视频'
  UNION ALL SELECT 'black','title','douyin','内置：视频/直播/短视频'
  UNION ALL SELECT 'black','title','抖音','内置：视频/直播/短视频'
  UNION ALL SELECT 'black','title','kuaishou','内置：视频/直播/短视频'
  UNION ALL SELECT 'black','title','快手','内置：视频/直播/短视频'
  UNION ALL SELECT 'black','title','tiktok','内置：视频/直播/短视频'
  UNION ALL SELECT 'black','title','youtube','内置：视频/直播/短视频'
  UNION ALL SELECT 'black','title','netflix','内置：视频/直播/短视频'
  UNION ALL SELECT 'black','title','iqiyi','内置：视频/直播/短视频'
  UNION ALL SELECT 'black','title','爱奇艺','内置：视频/直播/短视频'
  UNION ALL SELECT 'black','title','腾讯视频','内置：视频/直播/短视频'
  UNION ALL SELECT 'black','title','优酷','内置：视频/直播/短视频'
  UNION ALL SELECT 'black','title','斗鱼','内置：视频/直播/短视频'
  UNION ALL SELECT 'black','title','虎牙','内置：视频/直播/短视频'
) t
WHERE NOT EXISTS (
  SELECT 1
  FROM rules r
  WHERE r.rule_type = t.rule_type
    AND r.match_mode = t.match_mode
    AND r.match_value = t.match_value
)`
