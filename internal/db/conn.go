package db

import (
	"database/sql"
	"time"

	"github.com/go-sql-driver/mysql"
)

func Open(dsn string) (*sql.DB, error) {
	cfg, err := parseMySQLConfig(dsn)
	if err != nil {
		return nil, err
	}

	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(connector)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)

	return db, nil
}

func parseMySQLConfig(dsn string) (*mysql.Config, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	// sqlc 将 DATE、DATETIME 和 TIMESTAMP 字段生成成 time.Time。
	// 如果部署配置漏写 parseTime=true，Ping 仍会成功，但查询到日期数据时
	// rows.Scan 会因驱动返回 []byte 而失败。这里在连接层统一保证日期解析。
	cfg.ParseTime = true
	return cfg, nil
}
