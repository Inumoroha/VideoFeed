package router

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	app "github.com/inumoroha/video-feed/internal/application/playback"
	authjwt "github.com/inumoroha/video-feed/internal/auth/jwt"
	"github.com/inumoroha/video-feed/internal/config"
	domain "github.com/inumoroha/video-feed/internal/domain/playback"
	handler "github.com/inumoroha/video-feed/internal/handler/playback"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// playbackRouteRepository 模拟路由的报告创建、重放和数据库故障。
type playbackRouteRepository struct {
	report *domain.QoSReport
	fail   bool
}

// FindConfig 返回缺省配置或模拟数据库故障。
func (r *playbackRouteRepository) FindConfig(context.Context, string, string) (*domain.Config, error) {
	if r.fail {
		return nil, errors.New("private failure")
	}
	return nil, nil
}

// ListPreloadVideos 返回空列表以验证数组响应。
func (r *playbackRouteRepository) ListPreloadVideos(context.Context, int64, int) ([]*domain.PreloadVideo, error) {
	return nil, nil
}

// CreateQoSReport 保留首次请求以验证重复上报状态及认证用户来源。
func (r *playbackRouteRepository) CreateQoSReport(_ context.Context, report *domain.QoSReport) (*domain.QoSReport, bool, error) {
	if r.fail {
		return nil, false, errors.New("private failure")
	}
	if r.report != nil {
		return r.report, false, nil
	}
	r.report = report
	r.report.ID = 1
	return r.report, true, nil
}

// TestPlaybackRoutes 验证四条实际注册路由的认证、参数、用户隔离及创建和重放状态。
func TestPlaybackRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g := gin.New()
	repo := &playbackRouteRepository{}
	manager, err := authjwt.NewManager(config.JWTConfig{Secret: "playback-test-secret-at-least-32-bytes", AccessTTL: time.Hour, RefreshTTL: 2 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	token, err := manager.SignAccessToken(42, "user")
	if err != nil {
		t.Fatal(err)
	}
	registerPlaybackRoutes(g, manager, handler.New(app.New(repo)), "internal-test")
	cases := []struct {
		method, path, body, auth string
		status                   int
		contains                 string
	}{
		{"GET", "/api/playback-config", "", "", 401, "error"},
		{"GET", "/api/preload-videos", "", "", 401, "error"},
		{"POST", "/api/playback-qos-reports", "{}", "", 401, "error"},
		{"POST", "/internal/playback-qos-reports", "{}", "", 401, "error"},
		{"GET", "/api/playback-config", "", "jwt", 200, `"preload_count":3`},
		{"GET", "/api/playback-config?platform=abcdefghijklmnopq", "", "jwt", 400, "platform is invalid"},
		{"GET", "/api/playback-config?network_type=abcdefghijklmnopq", "", "jwt", 400, "network type is invalid"},
		{"GET", "/api/preload-videos", "", "jwt", 200, `"items":[]`},
		{"GET", "/api/preload-videos?current_video_id=-1", "", "jwt", 400, "video id"},
		{"GET", "/api/preload-videos?current_video_id=bad", "", "jwt", 400, "video id"},
		{"GET", "/api/preload-videos?limit=0", "", "jwt", 400, "limit"},
		{"GET", "/api/preload-videos?limit=bad", "", "jwt", 400, "limit"},
		{"GET", "/api/preload-videos?limit=99", "", "jwt", 200, "items"},
		{"POST", "/api/playback-qos-reports", "{", "jwt", 400, "invalid request"},
		{"POST", "/internal/playback-qos-reports", "{", "internal", 400, "invalid request"},
		{"POST", "/api/playback-qos-reports", `{"video_id":0}`, "jwt", 400, "video id"},
		{"POST", "/api/playback-qos-reports", `{"video_id":1,"first_frame_ms":-1}`, "jwt", 400, "first frame"},
		{"POST", "/api/playback-qos-reports", `{"video_id":1,"stutter_count":-1}`, "jwt", 400, "stutter"},
		{"POST", "/api/playback-qos-reports", `{"video_id":1,"watch_ms":-1}`, "jwt", 400, "watch"},
		{"POST", "/internal/playback-qos-reports", `{"video_id":1,"user_id":-1}`, "internal", 400, "user id"},
		{"POST", "/api/playback-qos-reports", `{"video_id":1,"user_id":999}`, "jwt", 201, `"user_id":42`},
		{"POST", "/api/playback-qos-reports", `{"video_id":1,"watch_ms":99}`, "jwt", 200, `"watch_ms":0`},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "same")
		if c.auth == "jwt" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if c.auth == "internal" {
			req.Header.Set("X-Internal-Token", "internal-test")
		}
		rec := httptest.NewRecorder()
		g.ServeHTTP(rec, req)
		if rec.Code != c.status || !strings.Contains(rec.Body.String(), c.contains) {
			t.Fatalf("%s %s: %d %s", c.method, c.path, rec.Code, rec.Body.String())
		}
	}
	repo.report = nil
	req := httptest.NewRequest("POST", "/internal/playback-qos-reports", strings.NewReader(`{"video_id":1}`))
	req.Header.Set("X-Internal-Token", "internal-test")
	rec := httptest.NewRecorder()
	g.ServeHTTP(rec, req)
	if rec.Code != 201 || repo.report.UserID != 0 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	repo.fail = true
	req = httptest.NewRequest("GET", "/api/playback-config", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	g.ServeHTTP(rec, req)
	if rec.Code != 500 || rec.Body.String() != `{"error":"internal server error"}` {
		t.Fatal(rec.Body.String())
	}
}
