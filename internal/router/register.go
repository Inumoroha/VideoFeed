package router

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/gin-gonic/gin"
	applicationfeed "github.com/inumoroha/video-feed/internal/application/feed"
	applicationinteraction "github.com/inumoroha/video-feed/internal/application/interaction"
	applicationrelation "github.com/inumoroha/video-feed/internal/application/relation"
	applicationuser "github.com/inumoroha/video-feed/internal/application/user"
	applicationvideo "github.com/inumoroha/video-feed/internal/application/video"
	authjwt "github.com/inumoroha/video-feed/internal/auth/jwt"
	"github.com/inumoroha/video-feed/internal/config"
	"github.com/inumoroha/video-feed/internal/handler"
	feedhandler "github.com/inumoroha/video-feed/internal/handler/feed"
	interactionhandler "github.com/inumoroha/video-feed/internal/handler/interaction"
	relationhandler "github.com/inumoroha/video-feed/internal/handler/relation"
	uploadhandler "github.com/inumoroha/video-feed/internal/handler/upload"
	userhandler "github.com/inumoroha/video-feed/internal/handler/user"
	videohandler "github.com/inumoroha/video-feed/internal/handler/video"
	feedcache "github.com/inumoroha/video-feed/internal/infrastructure/cache/feed"
	infraredis "github.com/inumoroha/video-feed/internal/infrastructure/cache/redis"
	inframq "github.com/inumoroha/video-feed/internal/infrastructure/mq"
	feedpersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/feed"
	interactionpersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/interaction"
	relationpersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/relation"
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
	if err := gormDB.AutoMigrate(&videopersistence.VideoStatModel{}); err != nil {
		return fmt.Errorf("migrate video stats: %w", err)
	}
	if err := gormDB.AutoMigrate(&interactionpersistence.ActionModel{}, &interactionpersistence.ActionIdempotencyModel{}, &interactionpersistence.CommentModel{}); err != nil {
		return fmt.Errorf("migrate interactions: %w", err)
	}
	if err := gormDB.AutoMigrate(&relationpersistence.FollowModel{}, &relationpersistence.RelationStatModel{}, &relationpersistence.FollowIdempotencyModel{}); err != nil {
		return fmt.Errorf("migrate relations: %w", err)
	}
	if err := videopersistence.EnsureStats(gormDB); err != nil {
		return fmt.Errorf("ensure video stats: %w", err)
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
	feedOptions := []applicationfeed.Option{}
	var feedCache *feedcache.FeedCache
	if cfg.Redis.Addr != "" {
		redisClient := infraredis.NewRedisClient(cfg.Redis)
		feedCache = feedcache.NewFeedCache(redisClient)
		feedOptions = append(feedOptions, applicationfeed.WithFeedCache(feedCache))
	}
	feedService := applicationfeed.NewService(feedRepository, feedOptions...)
	feedHandler := feedhandler.NewHandler(feedService)
	interactionRepository := interactionpersistence.New(gormDB)
	interactionOptions := []func(*applicationinteraction.Service){}
	if feedCache != nil {
		interactionOptions = append(interactionOptions, applicationinteraction.WithHotScoreCache(feedCache))
		if cfg.RabbitMQ.URL != "" {
			if broker, e := inframq.NewRabbitMQ(cfg.RabbitMQ); e == nil {
				interactionOptions = append(interactionOptions, applicationinteraction.WithAsyncActionPipeline(feedCache, broker))
				_ = applicationinteraction.NewActionWorker(interactionRepository, broker).Start(context.Background())
			}
		}
	}
	interactionService := applicationinteraction.NewService(interactionRepository, interactionOptions...)
	interactionHandler := interactionhandler.NewHandler(interactionService)
	relationRepository := relationpersistence.New(gormDB)
	relationService := applicationrelation.NewService(relationRepository)
	relationHandler := relationhandler.NewHandler(relationService)
	uploadRoot, err := filepath.Abs("./uploads")
	if err != nil {
		return fmt.Errorf("resolve upload directory: %w", err)
	}
	uploadHandler := uploadhandler.NewHandler(uploadRoot)
	g.GET("/health", handler.Health)
	g.Static("/uploads", uploadRoot)

	registerRoutes(g, userHandler, videoHandler, feedHandler, uploadHandler, jwtManager, interactionHandler, relationHandler)
	registerRelationRoutes(g, jwtManager, relationHandler)

	return nil
}

// registerRelationRoutes 注册需要认证的关注、取关和关系列表接口。
func registerRelationRoutes(g *gin.Engine, jwtManager *authjwt.Manager, relationHandler *relationhandler.Handler) {
	if relationHandler == nil {
		return
	}
	authMiddleware := middleware.NewJWTAuthMiddleware(jwtManager)
	api := g.Group("/api")
	relations := api.Group("/relations", authMiddleware)
	relations.PUT("/:targetUserId", relationHandler.Follow)
	relations.DELETE("/:targetUserId", relationHandler.Unfollow)
	relations.GET("/following", relationHandler.ListFollowing)
	relations.GET("/followers", relationHandler.ListFollowers)
}

// registerRoutes 注册认证、用户、视频、互动、信息流和上传接口；互动处理器为空时跳过互动路由。
func registerRoutes(g *gin.Engine, userHandler *userhandler.Handler, videoHandler *videohandler.Handler, feedHandler *feedhandler.Handler, uploadHandler *uploadhandler.Handler, jwtManager *authjwt.Manager, optionalHandlers ...any) {
	authMiddleware := middleware.NewJWTAuthMiddleware(jwtManager)
	optionalMiddleware := middleware.NewOptionalJWTAuth(jwtManager)
	api := g.Group("/api")
	var interactionHandler *interactionhandler.Handler
	var relationHandler *relationhandler.Handler
	for _, handler := range optionalHandlers {
		switch value := handler.(type) {
		case *interactionhandler.Handler:
			interactionHandler = value
		case *relationhandler.Handler:
			relationHandler = value
		}
	}

	sessions := api.Group("/sessions")
	sessions.POST("", userHandler.Login)
	sessions.DELETE("/current", authMiddleware, userHandler.Logout)

	users := api.Group("/users")
	users.POST("", userHandler.Register)
	users.GET("/me", authMiddleware, userHandler.Me)
	users.PATCH("/me", authMiddleware, userHandler.UpdateMe)
	users.GET("/:userId", userHandler.Get)
	users.GET("/me/videos", authMiddleware, videoHandler.ListMine)
	users.GET("/:userId/videos", videoHandler.ListByAuthor)
	if relationHandler != nil {
		users.PUT("/me/following/:targetUserId", authMiddleware, relationHandler.Follow)
		users.DELETE("/me/following/:targetUserId", authMiddleware, relationHandler.Unfollow)
		users.GET("/me/following", authMiddleware, relationHandler.ListFollowing)
		users.GET("/me/followers", authMiddleware, relationHandler.ListFollowers)
	}

	videos := api.Group("/videos")
	videos.POST("", authMiddleware, videoHandler.Create)
	videos.GET("/:videoId", videoHandler.Get)
	videos.DELETE("/:videoId", authMiddleware, videoHandler.Delete)

	api.GET("/feed-items", optionalMiddleware, feedHandler.ListFeedItems)
	api.POST("/feed/query", optionalMiddleware, feedHandler.Query)

	if interactionHandler != nil {
		videos.PUT("/:videoId/like", authMiddleware, interactionHandler.Like)
		videos.DELETE("/:videoId/like", authMiddleware, interactionHandler.Unlike)
		videos.PUT("/:videoId/favorite", authMiddleware, interactionHandler.Favorite)
		videos.DELETE("/:videoId/favorite", authMiddleware, interactionHandler.Unfavorite)
		videos.POST("/:videoId/comments", authMiddleware, interactionHandler.CreateComment)
		videos.GET("/:videoId/comments", interactionHandler.ListComments)
		api.DELETE("/comments/:commentId", authMiddleware, interactionHandler.DeleteComment)
	}

	uploads := api.Group("/uploads", authMiddleware)
	uploads.POST("", uploadHandler.Create)
}
