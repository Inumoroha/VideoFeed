package router

import (
	"fmt"
	applicationexposure "github.com/inumoroha/video-feed/internal/application/exposure"
	applicationmessage "github.com/inumoroha/video-feed/internal/application/message"
	applicationplayback "github.com/inumoroha/video-feed/internal/application/playback"
	applicationrecommendation "github.com/inumoroha/video-feed/internal/application/recommendation"
	exposurehandler "github.com/inumoroha/video-feed/internal/handler/exposure"
	messagehandler "github.com/inumoroha/video-feed/internal/handler/message"
	playbackhandler "github.com/inumoroha/video-feed/internal/handler/playback"
	recommendationhandler "github.com/inumoroha/video-feed/internal/handler/recommendation"
	exposurepersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/exposure"
	messagepersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/message"
	playbackpersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/playback"
	recommendationpersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/recommendation"
	"log"
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
	migration "github.com/inumoroha/video-feed/internal/infrastructure/persistence/migration"
	relationpersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/relation"
	userpersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/user"
	videopersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/video"
	"github.com/inumoroha/video-feed/internal/middleware"

	"gorm.io/gorm"
)

// Register 组装消息发布与请求处理依赖、迁移业务表并注册路由，不在 API 内启动消费者。
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

	if err := migration.AutoMigrate(gormDB); err != nil {
		return err
	}

	jwtManager, err := authjwt.NewManager(cfg.Auth.JWT)
	if err != nil {
		return fmt.Errorf("create JWT manager: %w", err)
	}

	userRepository := userpersistence.New(gormDB)
	userService := applicationuser.NewService(userRepository, jwtManager)
	userHandler := userhandler.NewHandler(userService)
	videoRepository := videopersistence.New(gormDB)
	exposureOptions := []applicationexposure.Option{}
	videoOptions := []applicationvideo.Option{}
	feedRepository := feedpersistence.New(gormDB)
	recommendationService := applicationrecommendation.New(recommendationpersistence.New(gormDB))
	feedOptions := []applicationfeed.Option{applicationfeed.WithRecommender(recommendationService)}
	var feedCache *feedcache.FeedCache
	if cfg.Redis.Addr != "" {
		redisClient := infraredis.NewRedisClient(cfg.Redis)
		feedCache = feedcache.NewFeedCache(redisClient)
		feedOptions = append(feedOptions, applicationfeed.WithFeedCache(feedCache))
	}
	feedService := applicationfeed.NewService(feedRepository, feedOptions...)
	feedHandler := feedhandler.NewHandler(feedService)
	interactionRepository := interactionpersistence.New(gormDB)
	messageService := applicationmessage.New(messagepersistence.New(gormDB))
	messageWriter := NewMessageWriter(messageService)
	interactionOptions := []func(*applicationinteraction.Service){applicationinteraction.WithMessageWriter(messageWriter)}
	if feedCache != nil {
		interactionOptions = append(interactionOptions, applicationinteraction.WithHotScoreCache(feedCache))
	}
	if cfg.RabbitMQ.URL != "" {
		broker, err := inframq.NewRabbitMQ(cfg.RabbitMQ)
		if err != nil {
			log.Printf("rabbitmq disabled: %v", err)
		} else {
			exposureOptions = append(exposureOptions, applicationexposure.WithViewEventPublisher(broker))
			videoOptions = append(videoOptions, applicationvideo.WithPublishedEventPublisher(broker))
			if feedCache != nil {
				interactionOptions = append(interactionOptions, applicationinteraction.WithAsyncActionPipeline(feedCache, broker))
			}
		}
	}
	exposureService := applicationexposure.NewService(exposurepersistence.New(gormDB), exposureOptions...)
	exposureHandler := exposurehandler.NewHandler(exposureService)
	videoService := applicationvideo.NewService(videoRepository, videoOptions...)
	videoHandler := videohandler.NewHandler(videoService)
	interactionService := applicationinteraction.NewService(interactionRepository, interactionOptions...)
	interactionHandler := interactionhandler.NewHandler(interactionService)
	relationRepository := relationpersistence.New(gormDB)
	relationOptions := []applicationrelation.Option{applicationrelation.WithMessageWriter(messageWriter)}
	if feedCache != nil {
		relationOptions = append(relationOptions, applicationrelation.WithFollowFeedBackfiller(NewFollowFeedBackfiller(feedRepository, feedCache)))
	}
	relationService := applicationrelation.NewService(relationRepository, relationOptions...)
	relationHandler := relationhandler.NewHandler(relationService)
	uploadRoot, err := filepath.Abs("./uploads")
	if err != nil {
		return fmt.Errorf("resolve upload directory: %w", err)
	}
	uploadHandler := uploadhandler.NewHandler(uploadRoot)
	g.GET("/health", handler.Health)
	g.Static("/uploads", uploadRoot)

	registerRoutes(g, userHandler, videoHandler, feedHandler, uploadHandler, jwtManager, interactionHandler, relationHandler, exposureHandler)
	registerRelationRoutes(g, jwtManager, relationHandler)
	registerRecommendationRoutes(g, recommendationhandler.New(recommendationService))
	registerMessageRoutes(g, jwtManager, messagehandler.New(messageService), cfg.Internal.Token)
	registerPlaybackRoutes(g, jwtManager, playbackhandler.New(applicationplayback.New(playbackpersistence.New(gormDB))), cfg.Internal.Token)

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

// registerRoutes 注册认证、用户、视频、信息流和上传接口，并按非空可选处理器注册互动、关系及曝光路由。
func registerRoutes(g *gin.Engine, userHandler *userhandler.Handler, videoHandler *videohandler.Handler, feedHandler *feedhandler.Handler, uploadHandler *uploadhandler.Handler, jwtManager *authjwt.Manager, optionalHandlers ...any) {
	authMiddleware := middleware.NewJWTAuthMiddleware(jwtManager)
	optionalMiddleware := middleware.NewOptionalJWTAuth(jwtManager)
	api := g.Group("/api")
	var exposureHandler *exposurehandler.Handler
	var interactionHandler *interactionhandler.Handler
	var relationHandler *relationhandler.Handler
	for _, handler := range optionalHandlers {
		switch value := handler.(type) {
		case *exposurehandler.Handler:
			exposureHandler = value
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
	if exposureHandler != nil {
		api.POST("/video-view-events", authMiddleware, exposureHandler.CreateViewEvent)
	}

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

// registerRecommendationRoutes 注册无需 JWT 的内部推荐候选、曝光决策与批量曝光接口。
func registerRecommendationRoutes(g *gin.Engine, handler *recommendationhandler.Handler) {
	internal := g.Group("/internal")
	internal.POST("/recommendation-candidates", handler.ListCandidates)
	internal.POST("/exposure-decisions", handler.DecideExposures)
	internal.POST("/exposures", handler.SaveExposures)
}
