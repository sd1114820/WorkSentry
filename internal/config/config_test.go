package config

import "testing"

func TestLoadExampleReportQueryTimeout(t *testing.T) {
	cfg, err := Load("../../config.example.yaml")
	if err != nil {
		t.Fatalf("加载示例配置失败: %v", err)
	}
	if cfg.Server.ReportQueryTimeoutSeconds != 5 {
		t.Fatalf("报表查询超时 = %d，期望 5", cfg.Server.ReportQueryTimeoutSeconds)
	}
}
