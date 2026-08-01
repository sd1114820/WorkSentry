CREATE TABLE IF NOT EXISTS schema_migrations (
  version BIGINT PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS departments (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  name VARCHAR(64) NOT NULL,
  parent_id BIGINT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS employees (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  employee_code VARCHAR(32) NOT NULL UNIQUE,
  name VARCHAR(64) NOT NULL,
  department_id BIGINT NULL,
  fingerprint_hash VARCHAR(128) NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  last_seen_at DATETIME NULL,
  last_status ENUM('work','normal','fish','idle','break','offline') NULL,
  last_description VARCHAR(255) NULL,
  current_segment_started_at DATETIME NULL,
  last_segment_end_at DATETIME NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_employees_department (department_id),
  INDEX idx_employees_last_seen (last_seen_at),
  INDEX idx_employees_enabled_last_seen (enabled, last_seen_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS admin_users (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  username VARCHAR(64) NOT NULL UNIQUE,
  password_hash VARCHAR(255) NOT NULL,
  display_name VARCHAR(64) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS admin_sessions (
  token VARCHAR(64) PRIMARY KEY,
  admin_id BIGINT NOT NULL,
  issued_at DATETIME NOT NULL,
  expires_at DATETIME NULL,
  revoked TINYINT(1) NOT NULL DEFAULT 0,
  last_seen DATETIME NULL,
  INDEX idx_admin_sessions_admin (admin_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS client_tokens (
  token VARCHAR(64) PRIMARY KEY,
  employee_id BIGINT NOT NULL,
  issued_at DATETIME NOT NULL,
  expires_at DATETIME NULL,
  revoked TINYINT(1) NOT NULL DEFAULT 0,
  last_seen DATETIME NULL,
  INDEX idx_client_tokens_employee (employee_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS rules (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  rule_type ENUM('white','black') NOT NULL,
  match_mode ENUM('process','title') NOT NULL,
  match_value VARCHAR(255) NOT NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  remark VARCHAR(255) NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS settings (
  id TINYINT PRIMARY KEY,
  idle_threshold_seconds INT NOT NULL DEFAULT 300,
  heartbeat_interval_seconds INT NOT NULL DEFAULT 300,
  offline_threshold_seconds INT NOT NULL DEFAULT 600,
  fish_ratio_warn_percent INT NOT NULL DEFAULT 10,
  update_policy TINYINT NOT NULL DEFAULT 0,
  latest_version VARCHAR(32) NULL,
  update_url VARCHAR(255) NULL,
  history_cleanup_enabled TINYINT NOT NULL DEFAULT 0,
  history_retention_days INT NOT NULL DEFAULT 40,
  history_cleanup_hour TINYINT NOT NULL DEFAULT 3,
  history_cleanup_last_run_at DATETIME NULL,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS raw_events (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  ingest_id CHAR(36) NULL,
  source_event_id VARCHAR(128) NULL,
  client_event_id VARCHAR(128) NULL,
  employee_id BIGINT NOT NULL,
  received_at DATETIME NOT NULL,
  process_name VARCHAR(255) NULL,
  window_title VARCHAR(512) NULL,
  idle_seconds INT NOT NULL,
  status ENUM('work','normal','fish','idle','break','offline') NOT NULL,
  client_version VARCHAR(32) NULL,
  ip_address VARCHAR(64) NULL,
  INDEX idx_raw_events_employee_time (employee_id, received_at),
  INDEX idx_raw_events_received_at (received_at, id),
  INDEX idx_raw_events_ingest (ingest_id),
  INDEX idx_raw_events_source_event (source_event_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS client_report_outbox (
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS processed_ingests (
  ingest_id CHAR(36) PRIMARY KEY,
  processed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS processed_source_events (
  source_event_id VARCHAR(128) PRIMARY KEY,
  first_ingest_id CHAR(36) NOT NULL,
  employee_id BIGINT NOT NULL,
  client_event_id VARCHAR(128) NOT NULL,
  processed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_processed_source_events_employee (employee_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS stat_deltas (
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS time_segments (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  employee_id BIGINT NOT NULL,
  start_at DATETIME NOT NULL,
  end_at DATETIME NOT NULL,
  status ENUM('work','normal','fish','idle','offline','incident','break') NOT NULL,
  description VARCHAR(255) NULL,
  source ENUM('system','offline','manual','incident') NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_time_segments_employee_time (employee_id, start_at),
  INDEX idx_time_segments_employee_end (employee_id, end_at, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS daily_stats (
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS manual_adjustments (
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS system_incidents (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  start_at DATETIME NOT NULL,
  end_at DATETIME NOT NULL,
  reason VARCHAR(64) NOT NULL,
  note TEXT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS audit_logs (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  operator_id BIGINT NOT NULL,
  action VARCHAR(64) NOT NULL,
  target_type VARCHAR(64) NOT NULL,
  target_id BIGINT NULL,
  detail JSON NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS work_sessions (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  employee_id BIGINT NOT NULL,
  start_at DATETIME NOT NULL,
  end_at DATETIME NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_work_sessions_employee_start (employee_id, start_at),
  INDEX idx_work_sessions_employee_end (employee_id, end_at),
  INDEX idx_work_sessions_end_employee_start (end_at, employee_id, start_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS checkout_templates (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  department_id BIGINT NOT NULL,
  name_zh VARCHAR(100) NOT NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_checkout_templates_department (department_id),
  INDEX idx_checkout_templates_enabled (enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS checkout_fields (
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS work_session_checkouts (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  work_session_id BIGINT NOT NULL,
  template_id BIGINT NOT NULL,
  template_snapshot_json TEXT NOT NULL,
  data_json TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_work_session_checkouts_session (work_session_id),
  INDEX idx_work_session_checkouts_template (template_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS department_work_rules (
  department_id BIGINT PRIMARY KEY,
  target_seconds INT NOT NULL DEFAULT 0,
  max_break_seconds INT NOT NULL DEFAULT 0,
  max_break_count INT NOT NULL DEFAULT 0,
  max_break_single_seconds INT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS department_status_thresholds (
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS work_session_reviews (
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
