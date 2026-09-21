package router

import (
	"github.com/gin-gonic/gin"
	authjwt "github.com/inumoroha/video-feed/internal/auth/jwt"
	playbackhandler "github.com/inumoroha/video-feed/internal/handler/playback"
	"github.com/inumoroha/video-feed/internal/middleware"
)

// registerPlaybackRoutes 注册三个登录态播放接口和独立内部令牌保护的质量上报接口。
func registerPlaybackRoutes(g *gin.Engine, manager *authjwt.Manager, handler *playbackhandler.Handler, token string) {
	auth := middleware.NewJWTAuthMiddleware(manager)
	g.GET("/api/playback-config", auth, handler.GetConfig)
	g.GET("/api/preload-videos", auth, handler.ListPreloadVideos)
	g.POST("/api/playback-qos-reports", auth, handler.CreateQoSReport)
	g.POST("/internal/playback-qos-reports", middleware.NewInternalTokenAuth(token), handler.CreateInternalQoSReport)
}
