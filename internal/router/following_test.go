package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	applicationfeed "github.com/inumoroha/video-feed/internal/application/feed"
	authjwt "github.com/inumoroha/video-feed/internal/auth/jwt"
	"github.com/inumoroha/video-feed/internal/config"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	feedhandler "github.com/inumoroha/video-feed/internal/handler/feed"
	uploadhandler "github.com/inumoroha/video-feed/internal/handler/upload"
	userhandler "github.com/inumoroha/video-feed/internal/handler/user"
	videohandler "github.com/inumoroha/video-feed/internal/handler/video"
)

// followingRouteRepository 记录路由传入的登录用户，其他仓储方法不得被空关注页调用。
type followingRouteRepository struct {
	domainfeed.Repository
	viewerID int64
}

// ListFollowingPage 记录认证用户并返回空关注页。
func (r *followingRouteRepository) ListFollowingPage(_ context.Context, viewerID int64, _ *domainfeed.TimelineCursor, _ int) ([]*domainfeed.FeedPageItem, error) {
	r.viewerID = viewerID
	return nil, nil
}

// TestFollowingRegisteredRoutes 验证真实路由装配保留本项目查询路径，并正确传递或拒绝用户身份。
func TestFollowingRegisteredRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	manager, err := authjwt.NewManager(config.JWTConfig{Secret: "following-router-test-secret-32-bytes", AccessTTL: time.Hour, RefreshTTL: 2 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	repo := &followingRouteRepository{}
	registerRoutes(engine, userhandler.NewHandler(nil), videohandler.NewHandler(nil), feedhandler.NewHandler(applicationfeed.NewService(repo)), uploadhandler.NewHandler(t.TempDir()), manager)
	token, err := manager.SignAccessToken(42, "user")
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		path, body := "/api/feed-items?scene=following", ""
		if method == http.MethodPost {
			path, body = "/api/feed/query", `{"scene":"following"}`
		}
		for _, authenticated := range []bool{false, true} {
			repo.viewerID = 0
			r := httptest.NewRequest(method, path, strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			want := http.StatusUnauthorized
			if authenticated {
				r.Header.Set("Authorization", "Bearer "+token)
				want = http.StatusOK
			}
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, r)
			if w.Code != want {
				t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
			}
			if authenticated && repo.viewerID != 42 {
				t.Fatalf("viewer=%d", repo.viewerID)
			}
			if !authenticated && repo.viewerID != 0 {
				t.Fatal("anonymous request reached repository")
			}
		}
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/feed-queries", strings.NewReader(`{}`)))
	if w.Code != http.StatusNotFound {
		t.Fatalf("legacy source route unexpectedly registered: %d", w.Code)
	}
}
