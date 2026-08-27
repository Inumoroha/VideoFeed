package router

import (
	"fmt"

	"github.com/gin-gonic/gin"
	applicationuser "github.com/inumoroha/video-feed/internal/application/user"
	authjwt "github.com/inumoroha/video-feed/internal/auth/jwt"
	"github.com/inumoroha/video-feed/internal/config"
	"github.com/inumoroha/video-feed/internal/handler"
	userhandler "github.com/inumoroha/video-feed/internal/handler/user"
	userpersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/user"
	"github.com/inumoroha/video-feed/internal/middleware"

	"gorm.io/gorm"
)

// Register 组装用户认证依赖、迁移用户数据表并注册应用路由。
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

	jwtManager, err := authjwt.NewManager(cfg.Auth.JWT)
	if err != nil {
		return fmt.Errorf("create JWT manager: %w", err)
	}

	userRepository := userpersistence.New(gormDB)
	userService := applicationuser.NewService(userRepository, jwtManager)
	userHandler := userhandler.NewHandler(userService)

	g.GET("/health", handler.Health)

	registerRoutes(g, userHandler, jwtManager)

	return nil
}

// registerRoutes 注册公开认证接口和经过 JWT 认证的用户接口。
func registerRoutes(g *gin.Engine, userHandler *userhandler.Handler, jwtManager *authjwt.Manager) {
	authMiddleware := middleware.NewJWTAuthMiddleware(jwtManager)
	api := g.Group("/api")

	auth := api.Group("/auth")
	auth.POST("/register", userHandler.Register)
	auth.POST("/login/password", userHandler.Login)
	auth.POST("/logout", authMiddleware, userHandler.Logout)

	users := api.Group("/users", authMiddleware)
	users.GET("/me", userHandler.Me)
	users.PATCH("/me", userHandler.UpdateMe)
}
