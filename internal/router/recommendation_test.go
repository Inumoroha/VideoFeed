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
	recommendation "github.com/inumoroha/video-feed/internal/application/recommendation"
	authjwt "github.com/inumoroha/video-feed/internal/auth/jwt"
	"github.com/inumoroha/video-feed/internal/config"
	domainrecommendation "github.com/inumoroha/video-feed/internal/domain/recommendation"
	feedhandler "github.com/inumoroha/video-feed/internal/handler/feed"
	recommendationhandler "github.com/inumoroha/video-feed/internal/handler/recommendation"
	uploadhandler "github.com/inumoroha/video-feed/internal/handler/upload"
	userhandler "github.com/inumoroha/video-feed/internal/handler/user"
	videohandler "github.com/inumoroha/video-feed/internal/handler/video"
)

// recommendationRouteRepository 返回空候选以测试真实路由和身份传递。
type recommendationRouteRepository struct {
	domainrecommendation.Repository
	viewerID int64
}

// ListCandidatePool 记录当前用户并返回空候选以避免向量查询。
func (r *recommendationRouteRepository) ListCandidatePool(_ context.Context, userID int64, _ int) ([]*domainrecommendation.Candidate, error) {
	r.viewerID = userID
	return []*domainrecommendation.Candidate{}, nil
}

// TestRecommendationRegisteredRoutes 验证推荐内部路由、Feed 查询路径和身份传递。
func TestRecommendationRegisteredRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	manager, err := authjwt.NewManager(config.JWTConfig{Secret: "recommend-router-test-secret-32-bytes", AccessTTL: time.Hour, RefreshTTL: 2 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	repo := &recommendationRouteRepository{}
	recommendationService := recommendation.New(repo)
	registerRoutes(engine, userhandler.NewHandler(nil), videohandler.NewHandler(nil), feedhandler.NewHandler(applicationfeed.NewService(&followingRouteRepository{}, applicationfeed.WithRecommender(recommendationService))), uploadhandler.NewHandler(t.TempDir()), manager)
	registerRecommendationRoutes(engine, recommendationhandler.New(recommendationService))
	for _, route := range []struct {
		path, body string
		status     int
	}{{"/internal/recommendation-candidates", `{"user_id":42,"scene":"recommend"}`, 200}, {"/internal/exposures", `{}`, 201}, {"/internal/exposure-decisions", `{"user_id":42,"scene":"recommend"}`, 200}} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, route.path, strings.NewReader(route.body))
		r.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, r)
		if w.Code != route.status {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	token, err := manager.SignAccessToken(42, "user")
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		path, body := "/api/feed-items?scene=recommend", ""
		if method == http.MethodPost {
			path, body = "/api/feed/query", `{"scene":"recommend"}`
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
