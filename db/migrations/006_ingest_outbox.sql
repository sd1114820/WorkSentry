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

ALTER TABLE raw_events
  ADD COLUMN ingest_id CHAR(36) NULL AFTER id,
  ADD COLUMN source_event_id VARCHAR(128) NULL AFTER ingest_id,
  ADD COLUMN client_event_id VARCHAR(128) NULL AFTER source_event_id,
  ADD INDEX idx_raw_events_ingest (ingest_id),
  ADD INDEX idx_raw_events_source_event (source_event_id);
