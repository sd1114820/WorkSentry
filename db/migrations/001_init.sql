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
  last_status ENUM('work','normal','fish','idle') NULL,
  last_description VARCHAR(255) NULL,
  last_segment_end_at DATETIME NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_employees_department (department_id),
  INDEX idx_employees_last_seen (last_seen_at)
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
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS raw_events (
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS time_segments (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  employee_id BIGINT NOT NULL,
  start_at DATETIME NOT NULL,
  end_at DATETIME NOT NULL,
  status ENUM('work','normal','fish','idle','offline','incident') NOT NULL,
  description VARCHAR(255) NULL,
  source ENUM('system','offline','manual','incident') NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_time_segments_employee_time (employee_id, start_at)
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

-- ============================================================
-- WorkSentry 内置黑名单规则（默认数据）
-- ============================================================
-- WorkSentry 内置黑名单规则（幂等）
SET NAMES utf8mb4;
-- 说明：仅在 rules 表不存在相同 rule_type + match_mode + match_value 时插入

INSERT INTO rules (rule_type, match_mode, match_value, enabled, remark)
SELECT t.rule_type, t.match_mode, t.match_value, 1, t.remark
FROM (
  -- 内置：游戏平台/游戏进程
  SELECT 'black' AS rule_type, 'process' AS match_mode, 'steam.exe' AS match_value, '内置：游戏平台/游戏进程' AS remark
  UNION ALL SELECT 'black','process','epicgameslauncher.exe','内置：游戏平台/游戏进程'
  UNION ALL SELECT 'black','process','battle.net.exe','内置：游戏平台/游戏进程'
  UNION ALL SELECT 'black','process','wegame.exe','内置：游戏平台/游戏进程'
  UNION ALL SELECT 'black','process','riotclientservices.exe','内置：游戏平台/游戏进程'
  UNION ALL SELECT 'black','process','leagueclient.exe','内置：游戏平台/游戏进程'
  UNION ALL SELECT 'black','process','valorant.exe','内置：游戏平台/游戏进程'
  UNION ALL SELECT 'black','process','genshinimpact.exe','内置：游戏平台/游戏进程'
  UNION ALL SELECT 'black','process','starrail.exe','内置：游戏平台/游戏进程'

  -- 内置：视频/直播/短视频（标题关键词，适用于浏览器/桌面客户端）
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
);



