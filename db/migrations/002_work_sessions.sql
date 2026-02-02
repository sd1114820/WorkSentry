CREATE TABLE IF NOT EXISTS work_sessions (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  employee_id BIGINT NOT NULL,
  start_at DATETIME NOT NULL,
  end_at DATETIME NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_work_sessions_employee_start (employee_id, start_at),
  INDEX idx_work_sessions_employee_end (employee_id, end_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
