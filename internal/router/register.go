package router

import (
	"github.com/gin-gonic/gin"
	"github.com/inumoroha/video-feed/internal/handler"
)

// Register 注册路由
func Register(g *gin.Engine) {
	g.GET("/health", handler.Health)
}
