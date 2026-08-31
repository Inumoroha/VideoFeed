package router

import (
	"fmt"

	"github.com/gin-gonic/gin"
	applicationfeed "github.com/inumoroha/video-feed/internal/application/feed"
	applicationuser "github.com/inumoroha/video-feed/internal/application/user"
	applicationvideo "github.com/inumoroha/video-feed/internal/application/video"
	authjwt "github.com/inumoroha/video-feed/internal/auth/jwt"
	"github.com/inumoroha/video-feed/internal/config"
	"github.com/inumoroha/video-feed/internal/handler"
	feedhandler "github.com/inumoroha/video-feed/internal/handler/feed"
	userhandler "github.com/inumoroha/video-feed/internal/handler/user"
	videohandler "github.com/inumoroha/video-feed/internal/handler/video"

	feedpersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/feed"
	userpersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/user"
	videopersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/video"
	"github.com/inumoroha/video-feed/internal/middleware"

	"gorm.io/gorm"
)

// Register 组装应用依赖、迁移业务数据表并注册全部应用路由。
func Register(g *gin.Engine, cfg *config.Config, gormDB *gorm.DB) error {
	if g == nil {
		return fmt.Errorf("gin engine is nil")
	}
	if cfg == nil {
		return fmt.Errorf("application config is nil")
	}
	if gormDB == nil {
		return fmt.Errorf("gorm database is nil")
	}

	if err := gormDB.AutoMigrate(&userpersistence.UserModel{}); err != nil {
		return fmt.Errorf("migrate users: %w", err)
	}
	if err := gormDB.AutoMigrate(&videopersistence.VideoModel{}); err != nil {
		return fmt.Errorf("migrate videos: %w", err)
	}

	if err := gormDB.AutoMigrate(&feedpersistence.FeedCursorModel{}, &feedpersistence.FeedViewEventModel{}); err != nil {
		return fmt.Errorf("migrate feed tables: %w", err)
	}

	jwtManager, err := authjwt.NewManager(cfg.Auth.JWT)
	if err != nil {
		return fmt.Errorf("create JWT manager: %w", err)
	}

	userRepository := userpersistence.New(gormDB)
	userService := applicationuser.NewService(userRepository, jwtManager)
	userHandler := userhandler.NewHandler(userService)
	videoRepository := videopersistence.New(gormDB)
	videoService := applicationvideo.NewService(videoRepository)
	videoHandler := videohandler.NewHandler(videoService)
	feedRepository := feedpersistence.New(gormDB)
	feedService := applicationfeed.NewService(feedRepository)
	feedHandler := feedhandler.NewHandler(feedService)

	g.GET("/health", handler.Health)

	registerRoutes(g, userHandler, videoHandler, feedHandler, jwtManager)

	return nil
}

// registerRoutes 注册认证、用户、视频和信息流接口。
func registerRoutes(g *gin.Engine, userHandler *userhandler.Handler, videoHandler *videohandler.Handler, feedHandler *feedhandler.Handler, jwtManager *authjwt.Manager) {
	authMiddleware := middleware.NewJWTAuthMiddleware(jwtManager)
	api := g.Group("/api")

	auth := api.Group("/auth")
	auth.POST("/register", userHandler.Register)
	auth.POST("/login/password", userHandler.Login)
	auth.POST("/logout", authMiddleware, userHandler.Logout)

	users := api.Group("/users", authMiddleware)
	users.GET("/me", userHandler.Me)
	users.PATCH("/me", userHandler.UpdateMe)

	videos := api.Group("/videos")
	videos.POST("", authMiddleware, videoHandler.Create)
	videos.GET("/mine", authMiddleware, videoHandler.ListMine)
	videos.GET("/:videoId", videoHandler.Get)
	videos.DELETE("/:videoId", authMiddleware, videoHandler.Delete)
	api.GET("/users/:userId/videos", videoHandler.ListByAuthor)

	feed := api.Group("/feed")
	feed.GET("/time", feedHandler.Time)
	feed.GET("/refresh", feedHandler.Refresh)
	feed.POST("/view-events", middleware.NewOptionalJWTAuthMiddleware(jwtManager), feedHandler.ReportViewEvent)
}
