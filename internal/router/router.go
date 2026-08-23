package router

import "github.com/gin-gonic/gin"

// Setup 设置 Gin 路由
func Setup() *gin.Engine {
	// 设置 Gin 模式
	gin.SetMode(gin.ReleaseMode)

	// 创建默认路由
	g := gin.Default()

	// 注册路由
	Register(g)

	return g
}
