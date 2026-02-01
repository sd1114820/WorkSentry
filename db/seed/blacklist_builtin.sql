-- WorkSentry 内置黑名单规则（幂等）
-- 说明：为避免通过 mysql 导入时字符集不一致导致中文备注乱码，强制使用 utf8mb4

SET NAMES utf8mb4;

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
