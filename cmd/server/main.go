package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"worksentry/internal/config"
	"worksentry/internal/db"
	httpserver "worksentry/internal/http"
	"worksentry/internal/http/handlers"
)

func main() {
	cfgPath := os.Getenv("WORKSENTRY_CONFIG")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		if cfgPath == "config.yaml" {
			if _, statErr := os.Stat("config.example.yaml"); statErr == nil {
				cfg, err = config.Load("config.example.yaml")
			}
		}
	}
	if err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}

	if cfg.App.Timezone != "" {
		if loc, tzErr := time.LoadLocation(cfg.App.Timezone); tzErr == nil {
			time.Local = loc
		}
	}

	sqlDB, err := db.Open(cfg.Database.DSN)
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	defer sqlDB.Close()

	h := handlers.NewHandler(cfg, sqlDB)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.StartBackgroundJobs(ctx)

	router := httpserver.NewRouter(h)

	srv := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      router,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeoutSeconds) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeoutSeconds) * time.Second,
	}

	log.Printf("服务启动: %s", cfg.Server.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("服务异常退出: %v", err)
	}
}
