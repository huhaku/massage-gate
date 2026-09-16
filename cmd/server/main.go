package main

import (
	"log"

	"github.com/gin-gonic/gin"

	"massagegate/internal/api"
	"massagegate/internal/config"
	"massagegate/internal/dispatch"
	"massagegate/internal/inbound"
	"massagegate/internal/store"
	"massagegate/internal/web"
)

func main() {
	cfg := config.Load()

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	// 默认全局参数
	store.SetSettingDefault(db, "keep_days", "30")
	store.SetSettingDefault(db, "timeout_default_ms", "10000")
	store.SetSettingDefault(db, "backoff_base_sec", "60")
	store.SetSettingDefault(db, "backoff_max_sec", "3600")

	// 上次进程中断遗留的"投递中"任务复位为待投递
	dispatch.ResetSending(db)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	inbound.Register(r, db)
	api.Register(r, db)
	web.Register(r)

	go dispatch.Run(db)

	log.Printf("massage-gate 启动: http://0.0.0.0%s (数据目录 %s)", cfg.Listen, cfg.DataDir)
	if err := r.Run(cfg.Listen); err != nil {
		log.Fatalf("HTTP 服务退出: %v", err)
	}
}
