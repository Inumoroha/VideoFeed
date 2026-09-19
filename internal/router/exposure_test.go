package router

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	applicationexposure "github.com/inumoroha/video-feed/internal/application/exposure"
	authjwt "github.com/inumoroha/video-feed/internal/auth/jwt"
	"github.com/inumoroha/video-feed/internal/config"
	domainexposure "github.com/inumoroha/video-feed/internal/domain/exposure"
	exposurehandler "github.com/inumoroha/video-feed/internal/handler/exposure"
	feedhandler "github.com/inumoroha/video-feed/internal/handler/feed"
	uploadhandler "github.com/inumoroha/video-feed/internal/handler/upload"
	userhandler "github.com/inumoroha/video-feed/internal/handler/user"
	videohandler "github.com/inumoroha/video-feed/internal/handler/video"
)

// exposureRouteRepository 记录真实注册路由传入的身份，并按测试需要模拟落库错误。
type exposureRouteRepository struct {
	user int64
	fail bool
}

// SaveViewEvent 记录身份并返回测试流水或内部存储错误。
func (r *exposureRouteRepository) SaveViewEvent(_ context.Context, event *domainexposure.ViewEvent) (*domainexposure.ViewEvent, *domainexposure.Exposure, error) {
	r.user = event.UserID
	if r.fail {
		return nil, nil, errors.New("private storage error")
	}
	event.ID = 1
	event.CreatedAt = time.Now()
	return event, nil, nil
}

// TestExposureRegisteredRoute 验证真实路由装配、JWT 用户身份优先于请求字段及内部错误响应。
func TestExposureRegisteredRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	manager, err := authjwt.NewManager(config.JWTConfig{Secret: "exposure-router-test-secret-32-bytes", AccessTTL: time.Hour, RefreshTTL: 2 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	repo := &exposureRouteRepository{}
	registerRoutes(engine, userhandler.NewHandler(nil), videohandler.NewHandler(nil), feedhandler.NewHandler(nil), uploadhandler.NewHandler(t.TempDir()), manager, exposurehandler.NewHandler(applicationexposure.NewService(repo)))
	token, err := manager.SignAccessToken(42, "user")
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []int{401, 201, 500} {
		repo.user = 0
		repo.fail = status == 500
		req := httptest.NewRequest(http.MethodPost, "/api/video-view-events", strings.NewReader(`{"user_id":999,"video_id":1001,"scene":"hot","event_type":"play"}`))
		req.Header.Set("Content-Type", "application/json")
		if status != 401 {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		if response.Code != status {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		if status == 401 && repo.user != 0 || status != 401 && repo.user != 42 {
			t.Fatal("认证身份错误")
		}
		if status == 500 && strings.TrimSpace(response.Body.String()) != `{"error":"internal server error"}` {
			t.Fatal(response.Body.String())
		}
	}
}
