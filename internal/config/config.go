package config

import (
	"errors"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	App      AppConfig      `yaml:"app"`
}

type ServerConfig struct {
	Addr                string `yaml:"addr"`
	ReadTimeoutSeconds  int    `yaml:"read_timeout_seconds"`
	WriteTimeoutSeconds int    `yaml:"write_timeout_seconds"`
	IdleTimeoutSeconds  int    `yaml:"idle_timeout_seconds"`
}

type DatabaseConfig struct {
	DSN string `yaml:"dsn"`
}

type AppConfig struct {
	Timezone    string      `yaml:"timezone"`
	Environment string      `yaml:"environment"`
	Admin       AdminConfig `yaml:"admin"`
}

type AdminConfig struct {
	Username    string `yaml:"username"`
	Password    string `yaml:"password"`
	DisplayName string `yaml:"display_name"`
}

func Load(path string) (*Config, error) {
	cfg := defaultConfig()

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return nil, err
	}

	if cfg.Server.Addr == "" {
		return nil, errors.New("服务地址不能为空")
	}
	if cfg.Database.DSN == "" {
		return nil, errors.New("数据库连接不能为空")
	}

	return &cfg, nil
}

func defaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Addr:                ":8080",
			ReadTimeoutSeconds:  15,
			WriteTimeoutSeconds: 15,
			IdleTimeoutSeconds:  60,
		},
		App: AppConfig{
			Timezone:    "Asia/Shanghai",
			Environment: "dev",
			Admin:       AdminConfig{},
		},
	}
}
