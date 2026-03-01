ALTER TABLE time_segments
  ADD INDEX idx_time_segments_employee_end (employee_id, end_at, id);

ALTER TABLE work_sessions
  ADD INDEX idx_work_sessions_end_employee_start (end_at, employee_id, start_at);

ALTER TABLE employees
  ADD INDEX idx_employees_enabled_last_seen (enabled, last_seen_at);
