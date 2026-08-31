package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	authjwt "github.com/inumoroha/video-feed/internal/auth/jwt"
	"github.com/inumoroha/video-feed/internal/config"
	feedhandler "github.com/inumoroha/video-feed/internal/handler/feed"
	userhandler "github.com/inumoroha/video-feed/internal/handler/user"
	videohandler "github.com/inumoroha/video-feed/internal/handler/video"
)

// TestSetupRejectsNilConfig 验证路由初始化会拒绝空应用配置。
func TestSetupRejectsNilConfig(t *testing.T) {
	engine, err := Setup(nil, nil)
	if err == nil {
		t.Fatal("Setup() error = nil, want non-nil")
	}
	if engine != nil {
		t.Fatal("Setup() engine is not nil on error")
	}
}

// TestSetupRejectsNilDatabase 验证路由初始化会拒绝空 GORM 数据库实例。
func TestSetupRejectsNilDatabase(t *testing.T) {
	engine, err := Setup(&config.Config{}, nil)
	if err == nil {
		t.Fatal("Setup() error = nil, want non-nil")
	}
	if engine != nil {
		t.Fatal("Setup() engine is not nil on error")
	}
}

// TestRegisterRoutesRegistersExpectedPaths 验证路由注册函数使用模块约定的路径和 HTTP 方法。
func TestRegisterRoutesRegistersExpectedPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	manager, err := authjwt.NewManager(config.JWTConfig{
		Secret:     "01234567890123456789012345678901",
		AccessTTL:  time.Hour,
		RefreshTTL: 2 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	registerRoutes(engine, userhandler.NewHandler(nil), videohandler.NewHandler(nil), feedhandler.NewHandler(nil), manager)
	routes := make(map[string]bool)
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = true
	}

	for _, route := range []string{
		http.MethodPost + " /api/auth/register",
		http.MethodPost + " /api/auth/login/password",
		http.MethodPost + " /api/auth/logout",
		http.MethodGet + " /api/users/me",
		http.MethodPatch + " /api/users/me",
		http.MethodPost + " /api/videos",
		http.MethodGet + " /api/videos/mine",
		http.MethodGet + " /api/videos/:videoId",
		http.MethodDelete + " /api/videos/:videoId",
		http.MethodGet + " /api/users/:userId/videos",
		http.MethodGet + " /api/feed/time",
		http.MethodGet + " /api/feed/refresh",
		http.MethodPost + " /api/feed/view-events",
	} {
		if !routes[route] {
			t.Errorf("missing route %q", route)
		}
	}
	if routes[http.MethodPost+" /api/auth/login"] {
		t.Error("unexpected legacy login route /api/auth/login")
	}
}

// TestRegisterRoutesProtectsUserEndpoints 验证用户资料接口缺少认证信息时返回 401。
func TestRegisterRoutesProtectsUserEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	manager, err := authjwt.NewManager(config.JWTConfig{
		Secret:     "01234567890123456789012345678901",
		AccessTTL:  time.Hour,
		RefreshTTL: 2 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	registerRoutes(engine, userhandler.NewHandler(nil), videohandler.NewHandler(nil), feedhandler.NewHandler(nil), manager)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/users/me status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}
