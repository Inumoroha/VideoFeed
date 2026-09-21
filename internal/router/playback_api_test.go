package router

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"encoding/json"
	applicationplayback "github.com/inumoroha/video-feed/internal/application/playback"
	infrajwt "github.com/inumoroha/video-feed/internal/auth/jwt"
	"github.com/inumoroha/video-feed/internal/config"
	domainplayback "github.com/inumoroha/video-feed/internal/domain/playback"
	interfaceshttpplayback "github.com/inumoroha/video-feed/internal/handler/playback"

	"github.com/gin-gonic/gin"
)

// playbackConfigAPIResponse 保存播放配置接口的完整响应字段。
type playbackConfigAPIResponse struct {
	ID           int64     `json:"id"`
	Platform     string    `json:"platform"`
	NetworkType  string    `json:"network_type"`
	PreloadCount int       `json:"preload_count"`
	BufferMs     int       `json:"buffer_ms"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// preloadVideoAPIResponse 保存单个预加载视频的资源字段。
type preloadVideoAPIResponse struct {
	VideoID  int64  `json:"video_id"`
	MediaURL string `json:"media_url"`
	CoverURL string `json:"cover_url"`
}

// preloadVideosAPIResponse 保存预加载接口返回的资源列表。
type preloadVideosAPIResponse struct {
	Items []preloadVideoAPIResponse `json:"items"`
}

// qosReportAPIResponse 保存质量报告响应及可空首帧指标。
type qosReportAPIResponse struct {
	ID           int64     `json:"id"`
	UserID       int64     `json:"user_id"`
	VideoID      int64     `json:"video_id"`
	FirstFrameMs *int      `json:"first_frame_ms"`
	StutterCount int       `json:"stutter_count"`
	WatchMs      int       `json:"watch_ms"`
	CreatedAt    time.Time `json:"created_at"`
}

// memoryPlaybackRepo 通过互斥锁保护测试配置、视频和幂等报告，隔离并发请求。
type memoryPlaybackRepo struct {
	mu        sync.Mutex
	nextID    int64
	configs   map[memoryPlaybackConfigKey]*domainplayback.Config
	videos    []*memoryPlaybackVideo
	reports   map[int64]*domainplayback.QoSReport
	byRequest map[memoryPlaybackReportKey]int64
}

// memoryPlaybackConfigKey 按平台和网络区分测试配置。
type memoryPlaybackConfigKey struct {
	Platform    string
	NetworkType string
}

// memoryPlaybackReportKey 按用户和请求键隔离幂等记录。
type memoryPlaybackReportKey struct {
	UserID int64
	Key    string
}

// memoryPlaybackVideo 保存预加载排序所需的资源和发布时间。
type memoryPlaybackVideo struct {
	VideoID     int64
	MediaURL    string
	CoverURL    string
	PublishedAt time.Time
}

// newMemoryPlaybackRepo 创建预置网络配置、四条视频和空报告索引的测试仓储。
func newMemoryPlaybackRepo() *memoryPlaybackRepo {
	now := time.Now().UTC()
	return &memoryPlaybackRepo{
		nextID: 1,
		configs: map[memoryPlaybackConfigKey]*domainplayback.Config{
			{Platform: "Web", NetworkType: "WiFi"}:                            domainplayback.RestoreConfig(7, "Web", "WiFi", 4, 900, now),
			{Platform: "Android", NetworkType: domainplayback.NetworkDefault}: domainplayback.RestoreConfig(8, "Android", domainplayback.NetworkDefault, 2, 1500, now),
		},
		videos: []*memoryPlaybackVideo{
			{VideoID: 103, MediaURL: "/uploads/103.mp4", CoverURL: "/uploads/103.jpg", PublishedAt: now.Add(3 * time.Minute)},
			{VideoID: 102, MediaURL: "/uploads/102.mp4", CoverURL: "/uploads/102.jpg", PublishedAt: now.Add(2 * time.Minute)},
			{VideoID: 101, MediaURL: "/uploads/101.mp4", CoverURL: "/uploads/101.jpg", PublishedAt: now.Add(time.Minute)},
			{VideoID: 100, MediaURL: "/uploads/100.mp4", CoverURL: "/uploads/100.jpg", PublishedAt: now},
		},
		reports:   map[int64]*domainplayback.QoSReport{},
		byRequest: map[memoryPlaybackReportKey]int64{},
	}
}

// FindConfig 持锁读取并复制配置，未命中时返回空值。
func (r *memoryPlaybackRepo) FindConfig(ctx context.Context, platform string, networkType string) (*domainplayback.Config, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	config := r.configs[memoryPlaybackConfigKey{Platform: platform, NetworkType: networkType}]
	if config == nil {
		return nil, nil
	}
	cloned := *config
	return &cloned, nil
}

// ListPreloadVideos 持锁复制视频列表并按发布时间和标识倒序返回游标之后的资源。
func (r *memoryPlaybackRepo) ListPreloadVideos(ctx context.Context, currentVideoID int64, limit int) ([]*domainplayback.PreloadVideo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	videos := make([]*memoryPlaybackVideo, 0, len(r.videos))
	videos = append(videos, r.videos...)
	sort.Slice(videos, func(i, j int) bool {
		if videos[i].PublishedAt.Equal(videos[j].PublishedAt) {
			return videos[i].VideoID > videos[j].VideoID
		}
		return videos[i].PublishedAt.After(videos[j].PublishedAt)
	})

	var current *memoryPlaybackVideo
	for _, video := range videos {
		if video.VideoID == currentVideoID {
			current = video
			break
		}
	}

	items := make([]*domainplayback.PreloadVideo, 0, limit)
	for _, video := range videos {
		if current != nil && (video.PublishedAt.After(current.PublishedAt) || video.PublishedAt.Equal(current.PublishedAt) && video.VideoID >= current.VideoID) {
			continue
		}
		items = append(items, domainplayback.RestorePreloadVideo(video.VideoID, video.MediaURL, video.CoverURL))
		if len(items) >= limit {
			break
		}
	}
	return items, nil
}

// CreateQoSReport 持锁原子创建或重放质量报告，并复制返回值以隔离调用方修改。
func (r *memoryPlaybackRepo) CreateQoSReport(ctx context.Context, report *domainplayback.QoSReport) (*domainplayback.QoSReport, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if report.IdempotencyKey != "" {
		if id, exists := r.byRequest[memoryPlaybackReportKey{UserID: report.UserID, Key: report.IdempotencyKey}]; exists {
			return cloneQoSReport(r.reports[id]), false, nil
		}
	}

	created := cloneQoSReport(report)
	created.ID = r.nextID
	r.nextID++
	if created.CreatedAt.IsZero() {
		created.CreatedAt = time.Now().UTC().Add(time.Duration(created.ID) * time.Millisecond)
	}
	r.reports[created.ID] = created
	if created.IdempotencyKey != "" {
		r.byRequest[memoryPlaybackReportKey{UserID: created.UserID, Key: created.IdempotencyKey}] = created.ID
	}
	return cloneQoSReport(created), true, nil
}

// cloneQoSReport 复制质量报告并深拷贝可空首帧指标。
func cloneQoSReport(report *domainplayback.QoSReport) *domainplayback.QoSReport {
	cloned := *report
	if report.FirstFrameMs != nil {
		firstFrameMs := *report.FirstFrameMs
		cloned.FirstFrameMs = &firstFrameMs
	}
	return &cloned
}

// TestPlaybackAPIFlow 验证精确配置、两级回退、预加载顺序、质量报告重放及内部上报。
func TestPlaybackAPIFlow(t *testing.T) {
	router, jwtManager := newPlaybackRouter(t)
	token := signTestToken(t, jwtManager, 42)

	configResponse := performJSONRequest(router, http.MethodGet, "/api/playback-config?platform=Web&network_type=WiFi", "", token)
	requireStatus(t, configResponse, http.StatusOK)

	var config playbackConfigAPIResponse
	decodeJSON(t, configResponse, &config)
	if config.ID != 7 || config.PreloadCount != 4 || config.BufferMs != 900 {
		t.Fatalf("unexpected playback config: %+v", config)
	}

	defaultConfigResponse := performJSONRequest(router, http.MethodGet, "/api/playback-config?platform=Web&network_type=4G", "", token)
	requireStatus(t, defaultConfigResponse, http.StatusOK)

	decodeJSON(t, defaultConfigResponse, &config)
	if config.ID != 0 || config.PreloadCount != domainplayback.DefaultPreloadCount || config.BufferMs != domainplayback.DefaultBufferMs {
		t.Fatalf("expected default playback config, got %+v", config)
	}

	fallbackConfigResponse := performJSONRequest(router, http.MethodGet, "/api/playback-config?platform=Android&network_type=5G", "", token)
	requireStatus(t, fallbackConfigResponse, http.StatusOK)

	decodeJSON(t, fallbackConfigResponse, &config)
	if config.ID != 8 || config.PreloadCount != 2 || config.BufferMs != 1500 {
		t.Fatalf("expected network fallback playback config, got %+v", config)
	}

	preloadResponse := performJSONRequest(router, http.MethodGet, "/api/preload-videos?current_video_id=103&limit=2", "", token)
	requireStatus(t, preloadResponse, http.StatusOK)

	var preload preloadVideosAPIResponse
	decodeJSON(t, preloadResponse, &preload)
	if len(preload.Items) != 2 || preload.Items[0].VideoID != 102 || preload.Items[1].VideoID != 101 {
		t.Fatalf("unexpected preload items: %+v", preload.Items)
	}

	firstFrameMs := 186
	qosResponse := performPlaybackJSONRequest(
		router,
		http.MethodPost,
		"/api/playback-qos-reports",
		`{"video_id":103,"first_frame_ms":186,"stutter_count":1,"watch_ms":2400}`,
		token,
		"qos-1",
	)
	requireStatus(t, qosResponse, http.StatusCreated)

	var qos qosReportAPIResponse
	decodeJSON(t, qosResponse, &qos)
	if qos.UserID != 42 || qos.VideoID != 103 || qos.FirstFrameMs == nil || *qos.FirstFrameMs != firstFrameMs || qos.StutterCount != 1 {
		t.Fatalf("unexpected qos report: %+v", qos)
	}

	replayResponse := performPlaybackJSONRequest(
		router,
		http.MethodPost,
		"/api/playback-qos-reports",
		`{"video_id":103,"first_frame_ms":999,"stutter_count":9,"watch_ms":9999}`,
		token,
		"qos-1",
	)
	requireStatus(t, replayResponse, http.StatusOK)

	var replay qosReportAPIResponse
	decodeJSON(t, replayResponse, &replay)
	if replay.ID != qos.ID || replay.FirstFrameMs == nil || *replay.FirstFrameMs != firstFrameMs {
		t.Fatalf("expected idempotent qos replay, got %+v", replay)
	}

	internalResponse := performInternalPlaybackRequest(
		router,
		http.MethodPost,
		"/internal/playback-qos-reports",
		`{"user_id":77,"video_id":101,"stutter_count":0,"watch_ms":1200}`,
		testInternalToken,
		"qos-internal-1",
	)
	requireStatus(t, internalResponse, http.StatusCreated)
}

// TestPlaybackAPIValidation 验证播放接口的认证、参数边界和内部令牌校验。
func TestPlaybackAPIValidation(t *testing.T) {
	router, jwtManager := newPlaybackRouter(t)
	token := signTestToken(t, jwtManager, 42)

	requireStatus(t, performJSONRequest(router, http.MethodGet, "/api/playback-config", "", ""), http.StatusUnauthorized)
	requireStatus(t, performJSONRequest(router, http.MethodGet, "/api/preload-videos", "", ""), http.StatusUnauthorized)
	requireStatus(t, performPlaybackJSONRequest(router, http.MethodPost, "/api/playback-qos-reports", `{"video_id":103}`, "", ""), http.StatusUnauthorized)

	requireStatus(t, performJSONRequest(router, http.MethodGet, "/api/playback-config?platform="+strings.Repeat("x", 17), "", token), http.StatusBadRequest)
	requireStatus(t, performJSONRequest(router, http.MethodGet, "/api/preload-videos?limit=0", "", token), http.StatusBadRequest)
	requireStatus(t, performJSONRequest(router, http.MethodGet, "/api/preload-videos?current_video_id=bad", "", token), http.StatusBadRequest)
	requireStatus(t, performPlaybackJSONRequest(router, http.MethodPost, "/api/playback-qos-reports", `{"video_id":0}`, token, ""), http.StatusBadRequest)
	requireStatus(t, performPlaybackJSONRequest(router, http.MethodPost, "/api/playback-qos-reports", `{"video_id":103,"first_frame_ms":-1}`, token, ""), http.StatusBadRequest)
	requireStatus(t, performPlaybackJSONRequest(router, http.MethodPost, "/api/playback-qos-reports", `{"video_id":103,"stutter_count":-1}`, token, ""), http.StatusBadRequest)
	requireStatus(t, performPlaybackJSONRequest(router, http.MethodPost, "/api/playback-qos-reports", `{"video_id":103,"watch_ms":-1}`, token, ""), http.StatusBadRequest)
	requireStatus(t, performInternalPlaybackRequest(router, http.MethodPost, "/internal/playback-qos-reports", `{"user_id":77,"video_id":101}`, "", ""), http.StatusUnauthorized)
	requireStatus(t, performInternalPlaybackRequest(router, http.MethodPost, "/internal/playback-qos-reports", `{"user_id":77,"video_id":101}`, "wrong-token", ""), http.StatusUnauthorized)
	requireStatus(t, performInternalPlaybackRequest(router, http.MethodPost, "/internal/playback-qos-reports", `{"user_id":-1,"video_id":101}`, testInternalToken, ""), http.StatusBadRequest)
}

// newPlaybackRouter 使用内存仓储和实际播放路由注册函数创建带认证的测试引擎。
func newPlaybackRouter(t *testing.T) (*gin.Engine, *infrajwt.Manager) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	repo := newMemoryPlaybackRepo()
	service := applicationplayback.New(repo)
	handler := interfaceshttpplayback.New(service)
	jwtManager, err := infrajwt.NewManager(config.JWTConfig{Secret: "playback-api-test-secret-at-least-32-bytes", AccessTTL: 15 * time.Minute, RefreshTTL: 24 * time.Hour})
	if err != nil {
		t.Fatalf("new jwt manager: %v", err)
	}

	router := gin.New()
	registerPlaybackRoutes(router, jwtManager, handler, testInternalToken)

	return router, jwtManager
}

// performPlaybackJSONRequest 发送可携带访问令牌与幂等键的请求并返回记录响应。
func performPlaybackJSONRequest(router *gin.Engine, method, path, body, accessToken, idempotencyKey string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

// performInternalPlaybackRequest 发送可携带内部令牌与幂等键的请求并返回记录响应。
func performInternalPlaybackRequest(router *gin.Engine, method, path, body, internalToken, idempotencyKey string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if internalToken != "" {
		req.Header.Set("X-Internal-Token", internalToken)
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

const testInternalToken = "test-internal-token"

// signTestToken 为指定测试用户签发当前仓库的访问令牌。
func signTestToken(t *testing.T, manager *infrajwt.Manager, userID int64) string {
	t.Helper()
	token, err := manager.SignAccessToken(userID, "user")
	if err != nil {
		t.Fatalf("sign test token: %v", err)
	}
	return token
}

// performJSONRequest 发送不附带幂等键的播放接口请求。
func performJSONRequest(router *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	return performPlaybackJSONRequest(router, method, path, body, token, "")
}

// requireStatus 断言响应状态，失败时输出正文以定位问题。
func requireStatus(t *testing.T, resp *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if resp.Code != expected {
		t.Fatalf("expected status %d, got %d body=%s", expected, resp.Code, resp.Body.String())
	}
}

// decodeJSON 解码测试响应并在失败时输出原始正文。
func decodeJSON(t *testing.T, resp *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(resp.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response body %q: %v", resp.Body.String(), err)
	}
}

// TestPlaybackAPIConcurrentReplay 验证并发同键请求只创建一条报告、重放内容不变，并保持跨用户和空键隔离。
func TestPlaybackAPIConcurrentReplay(t *testing.T) {
	router, manager := newPlaybackRouter(t)
	token := signTestToken(t, manager, 42)
	responses := make(chan *httptest.ResponseRecorder, 24)
	var wg sync.WaitGroup
	for i := 0; i < cap(responses); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			responses <- performPlaybackJSONRequest(router, http.MethodPost, "/api/playback-qos-reports", `{"video_id":103,"first_frame_ms":186,"watch_ms":2400}`, token, "concurrent")
		}()
	}
	wg.Wait()
	close(responses)
	created := 0
	var reportID int64
	for resp := range responses {
		if resp.Code == http.StatusCreated {
			created++
		} else {
			requireStatus(t, resp, http.StatusOK)
		}
		var report qosReportAPIResponse
		decodeJSON(t, resp, &report)
		if reportID == 0 {
			reportID = report.ID
		}
		if report.ID != reportID || report.UserID != 42 || report.FirstFrameMs == nil || *report.FirstFrameMs != 186 || report.WatchMs != 2400 {
			t.Fatalf("unexpected concurrent report: %+v", report)
		}
	}
	if created != 1 {
		t.Fatalf("expected one creation, got %d", created)
	}
	replay := performPlaybackJSONRequest(router, http.MethodPost, "/api/playback-qos-reports", `{"video_id":100,"watch_ms":999}`, token, "concurrent")
	requireStatus(t, replay, http.StatusOK)
	var report qosReportAPIResponse
	decodeJSON(t, replay, &report)
	if report.ID != reportID || report.VideoID != 103 || report.WatchMs != 2400 {
		t.Fatalf("replay changed original: %+v", report)
	}
	other := performPlaybackJSONRequest(router, http.MethodPost, "/api/playback-qos-reports", `{"video_id":103}`, signTestToken(t, manager, 77), "concurrent")
	requireStatus(t, other, http.StatusCreated)
	decodeJSON(t, other, &report)
	if report.ID == reportID || report.UserID != 77 {
		t.Fatalf("user isolation failed: %+v", report)
	}
	for i := 0; i < 2; i++ {
		requireStatus(t, performPlaybackJSONRequest(router, http.MethodPost, "/api/playback-qos-reports", `{"video_id":103}`, token, ""), http.StatusCreated)
	}
}

// TestMemoryPlaybackIsolationAndCursor 验证测试仓储的深拷贝、相同时间游标排序和不存在游标回退。
func TestMemoryPlaybackIsolationAndCursor(t *testing.T) {
	repo := newMemoryPlaybackRepo()
	ctx := context.Background()
	cfg, err := repo.FindConfig(ctx, "Web", "WiFi")
	if err != nil {
		t.Fatal(err)
	}
	cfg.PreloadCount = 99
	cfg, _ = repo.FindConfig(ctx, "Web", "WiFi")
	if cfg.PreloadCount != 4 {
		t.Fatal("configuration mutation leaked")
	}
	frame := 186
	input := &domainplayback.QoSReport{UserID: 42, VideoID: 103, FirstFrameMs: &frame, IdempotencyKey: "clone"}
	report, _, err := repo.CreateQoSReport(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	frame = 999
	*report.FirstFrameMs = 777
	replay, created, err := repo.CreateQoSReport(ctx, input)
	if err != nil || created || *replay.FirstFrameMs != 186 {
		t.Fatalf("report mutation leaked: %+v %v", replay, err)
	}
	for _, video := range repo.videos {
		video.PublishedAt = time.Unix(1000, 0)
	}
	items, err := repo.ListPreloadVideos(ctx, 102, 2)
	if err != nil || len(items) != 2 || items[0].VideoID != 101 || items[1].VideoID != 100 {
		t.Fatalf("cursor: %+v %v", items, err)
	}
	items, err = repo.ListPreloadVideos(ctx, 999, 2)
	if err != nil || len(items) != 2 || items[0].VideoID != 103 || items[1].VideoID != 102 {
		t.Fatalf("fallback: %+v %v", items, err)
	}
}
