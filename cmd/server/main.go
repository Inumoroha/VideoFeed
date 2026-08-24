package main

import (
	"context"
	"log"

	"github.com/inumoroha/video-feed/internal/config"
	"github.com/inumoroha/video-feed/internal/database"
	"github.com/inumoroha/video-feed/internal/router"
	"github.com/inumoroha/video-feed/internal/server"
)

// main 加载应用配置、注册路由并启动 HTTP 服务。
func main() {
	// 1. 加载配置
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	// 2. 连接数据库
	db, err := database.New(context.Background(), cfg.Database)
	if err != nil {
		log.Fatalf("connecting database: %v", err)
	}
	defer db.Close()

	// 3. 设置 Gin 路由
	g := router.Setup()

	// 4. 启动服务器
	if err := server.Run(&cfg.Server, g); err != nil {
		log.Fatalf("starting server: %v", err)
	}
}
