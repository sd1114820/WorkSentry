package db

import (
	"testing"
)

func TestParseMySQLConfigEnablesTimeParsing(t *testing.T) {
	cfg, err := parseMySQLConfig("user:password@tcp(127.0.0.1:3306)/worksentry?charset=utf8mb4")
	if err != nil {
		t.Fatalf("解析测试数据库连接失败: %v", err)
	}
	if !cfg.ParseTime {
		t.Fatal("数据库连接层必须强制启用日期解析")
	}
}

func TestParseMySQLConfigRejectsInvalidDSN(t *testing.T) {
	if _, err := parseMySQLConfig("://"); err == nil {
		t.Fatal("非法数据库连接字符串必须返回错误")
	}
}
