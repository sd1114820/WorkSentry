ALTER TABLE employees
  MODIFY last_status ENUM('work','normal','fish','idle','break') NULL;

ALTER TABLE raw_events
  MODIFY status ENUM('work','normal','fish','idle','break') NOT NULL;

ALTER TABLE time_segments
  MODIFY status ENUM('work','normal','fish','idle','offline','incident','break') NOT NULL;

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
