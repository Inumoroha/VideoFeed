package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/inumoroha/video-feed/internal/auth/jwt"
	"github.com/inumoroha/video-feed/internal/config"
)

// TestNewJWTAuthMiddlewareRejectsNilManager 验证 JWT 管理器为空时返回服务错误。
func TestNewJWTAuthMiddlewareRejectsNilManager(t *testing.T) {
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = httptest.NewRequest(http.MethodGet, "/protected", nil)

	NewJWTAuthMiddleware(nil)(ginContext)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	assertErrorResponse(t, recorder, "authentication service unavailable")
}

// TestNewJWTAuthMiddlewareUsesErrorResponse 验证缺少认证头时使用统一错误响应格式。
func TestNewJWTAuthMiddlewareUsesErrorResponse(t *testing.T) {
	manager := newTestJWTManager(t)
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = httptest.NewRequest(http.MethodGet, "/protected", nil)

	NewJWTAuthMiddleware(manager)(ginContext)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	assertErrorResponse(t, recorder, "authorization header is required")
}

// TestNewJWTAuthMiddlewareSetsUserID 验证合法访问令牌的用户 ID 会写入 Gin 上下文。
func TestNewJWTAuthMiddlewareSetsUserID(t *testing.T) {
	manager := newTestJWTManager(t)
	token, err := manager.SignAccessToken(42, "user")
	if err != nil {
		t.Fatalf("SignAccessToken() error = %v", err)
	}
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = httptest.NewRequest(http.MethodGet, "/protected", nil)
	ginContext.Request.Header.Set("Authorization", "Bearer "+token)

	NewJWTAuthMiddleware(manager)(ginContext)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	userID, exists := ginContext.Get(ContextUserIDKey)
	if !exists || userID != int64(42) {
		t.Fatalf("context user ID = %v, exists = %t, want 42 and true", userID, exists)
	}
	role, exists := ginContext.Get(ContextUserRoleKey)
	if !exists || role != "user" {
		t.Fatalf("context user role = %v, exists = %t, want user and true", role, exists)
	}
}

// TestNewOptionalJWTAuthMiddlewareAllowsGuest 验证可选认证中间件允许无令牌访客请求继续执行。
func TestNewOptionalJWTAuthMiddlewareAllowsGuest(t *testing.T) {
	manager := newTestJWTManager(t)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(NewOptionalJWTAuthMiddleware(manager))
	engine.GET("/feed", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/feed", nil)
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

// TestNewOptionalJWTAuthMiddlewareSetsUserID 验证可选认证中间件识别合法令牌并写入用户 ID。
func TestNewOptionalJWTAuthMiddlewareSetsUserID(t *testing.T) {
	manager := newTestJWTManager(t)
	token, err := manager.SignAccessToken(42, "user")
	if err != nil {
		t.Fatalf("SignAccessToken() error = %v", err)
	}
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = httptest.NewRequest(http.MethodGet, "/feed", nil)
	ginContext.Request.Header.Set("Authorization", "Bearer "+token)

	NewOptionalJWTAuthMiddleware(manager)(ginContext)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	userID, exists := ginContext.Get(ContextUserIDKey)
	if !exists || userID != int64(42) {
		t.Fatalf("context user ID = %v, exists = %t, want 42 and true", userID, exists)
	}
}

// TestNewOptionalJWTAuthMiddlewareRejectsInvalidToken 验证可选认证中间件拒绝无效令牌。
func TestNewOptionalJWTAuthMiddlewareRejectsInvalidToken(t *testing.T) {
	manager := newTestJWTManager(t)
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = httptest.NewRequest(http.MethodGet, "/feed", nil)
	ginContext.Request.Header.Set("Authorization", "Bearer invalid")

	NewOptionalJWTAuthMiddleware(manager)(ginContext)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

// newTestJWTManager 创建测试所需的 JWT 管理器。
func newTestJWTManager(t *testing.T) *jwt.Manager {
	t.Helper()
	manager, err := jwt.NewManager(config.JWTConfig{
		Secret:     "test-secret-with-at-least-32-characters",
		AccessTTL:  time.Hour,
		RefreshTTL: 2 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	return manager
}

// assertErrorResponse 验证响应包含指定的统一错误字段。
func assertErrorResponse(t *testing.T, recorder *httptest.ResponseRecorder, want string) {
	t.Helper()
	var response map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["error"] != want {
		t.Fatalf("error = %q, want %q", response["error"], want)
	}
}
