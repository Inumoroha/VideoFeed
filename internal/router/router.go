package router

import (
	"github.com/gin-gonic/gin"
	"github.com/inumoroha/video-feed/internal/config"

	"gorm.io/gorm"
)

// Setup 创建 Gin 引擎并注册依赖应用配置和数据库的路由。
func Setup(cfg *config.Config, gormDB *gorm.DB) (*gin.Engine, error) {
	// 设置 Gin 模式
	gin.SetMode(gin.ReleaseMode)

	// 创建默认路由
	g := gin.Default()

	// 注册路由
	if err := Register(g, cfg, gormDB); err != nil {
		return nil, err
	}

	return g, nil
}
