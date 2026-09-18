package video_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	applicationvideo "github.com/inumoroha/video-feed/internal/application/video"
	authjwt "github.com/inumoroha/video-feed/internal/auth/jwt"
	"github.com/inumoroha/video-feed/internal/config"
	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"
	videohandler "github.com/inumoroha/video-feed/internal/handler/video"
	"github.com/inumoroha/video-feed/internal/middleware"

	"github.com/gin-gonic/gin"
)

// videoAPIResponse 表示视频 API 流程测试中解析的视频响应。
type videoAPIResponse struct {
	ID            int64      `json:"id"`
	AuthorID      int64      `json:"author_id"`
	Title         string     `json:"title"`
	Description   string     `json:"description"`
	MediaURL      string     `json:"media_url"`
	CoverURL      string     `json:"cover_url"`
	Status        int        `json:"status"`
	LikeCount     int        `json:"like_count"`
	CommentCount  int        `json:"comment_count"`
	FavoriteCount int        `json:"favorite_count"`
	PublishedAt   *time.Time `json:"published_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// videoListAPIResponse 表示视频列表 API 流程测试中解析的分页响应。
type videoListAPIResponse struct {
	Items  []videoAPIResponse `json:"items"`
	Limit  int                `json:"limit"`
	Offset int                `json:"offset"`
}

// memoryVideoRepo 以互斥锁维护测试视频与作者幂等键索引。
type memoryVideoRepo struct {
	mu            sync.Mutex
	nextID        int64
	byID          map[int64]*domainvideo.Video
	byIdempotency map[string]int64
}

// newMemoryVideoRepo 创建用于 HTTP 流程测试的内存视频仓储。
func newMemoryVideoRepo() *memoryVideoRepo {
	return &memoryVideoRepo{
		nextID:        1,
		byID:          map[int64]*domainvideo.Video{},
		byIdempotency: map[string]int64{},
	}
}

// Save 保存视频并模拟数据库生成 ID 及时间字段。
func (r *memoryVideoRepo) Save(_ context.Context, video *domainvideo.Video) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if video.IdempotencyKey != "" {
		key := memoryVideoIdempotencyKey(video.AuthorID, video.IdempotencyKey)
		if _, exists := r.byIdempotency[key]; exists {
			return domainvideo.ErrDuplicateIdempotencyKey
		}
	}

	now := time.Now()
	video.ID = r.nextID
	r.nextID++
	if video.CreatedAt.IsZero() {
		video.CreatedAt = now
	}
	video.UpdatedAt = now
	r.byID[video.ID] = cloneVideo(video)
	if video.IdempotencyKey != "" {
		r.byIdempotency[memoryVideoIdempotencyKey(video.AuthorID, video.IdempotencyKey)] = video.ID
	}
	return nil
}

// FindByID 返回仍处于已发布状态的视频。
func (r *memoryVideoRepo) FindByID(_ context.Context, id int64) (*domainvideo.Video, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	video, exists := r.byID[id]
	if !exists || video.Status != domainvideo.StatusPublished {
		return nil, domainvideo.ErrVideoNotFound
	}
	return cloneVideo(video), nil
}

// FindByIDAnyStatus 返回指定视频，不限制视频当前状态。
func (r *memoryVideoRepo) FindByIDAnyStatus(_ context.Context, id int64) (*domainvideo.Video, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	video, exists := r.byID[id]
	if !exists {
		return nil, domainvideo.ErrVideoNotFound
	}
	return cloneVideo(video), nil
}

// FindByAuthorAndIdempotencyKey 按作者和幂等键查找已保存的视频。
func (r *memoryVideoRepo) FindByAuthorAndIdempotencyKey(_ context.Context, authorID int64, key string) (*domainvideo.Video, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id, exists := r.byIdempotency[memoryVideoIdempotencyKey(authorID, key)]
	if !exists {
		return nil, domainvideo.ErrVideoNotFound
	}
	return cloneVideo(r.byID[id]), nil
}

// ListByAuthor 按发布时间倒序返回作者的已发布视频。
func (r *memoryVideoRepo) ListByAuthor(_ context.Context, authorID int64, limit, offset int) ([]*domainvideo.Video, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	videos := make([]*domainvideo.Video, 0)
	for _, video := range r.byID {
		if video.AuthorID == authorID && video.Status == domainvideo.StatusPublished {
			videos = append(videos, cloneVideo(video))
		}
	}
	sort.Slice(videos, func(i, j int) bool {
		left := publishedAtUnix(videos[i])
		right := publishedAtUnix(videos[j])
		if left == right {
			return videos[i].ID > videos[j].ID
		}
		return left > right
	})

	if offset >= len(videos) {
		return []*domainvideo.Video{}, nil
	}
	end := offset + limit
	if end > len(videos) {
		end = len(videos)
	}
	return videos[offset:end], nil
}

// UpdateStatus 更新内存仓储中的视频状态。
func (r *memoryVideoRepo) UpdateStatus(_ context.Context, video *domainvideo.Video) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	stored, exists := r.byID[video.ID]
	if !exists {
		return domainvideo.ErrVideoNotFound
	}
	stored.Status = video.Status
	stored.UpdatedAt = time.Now()
	return nil
}

// TestVideoAPIFlow 验证视频创建、幂等重放、查询、列表和删除的完整 HTTP 流程。
func TestVideoAPIFlow(t *testing.T) {
	router, jwtManager := newVideoRouter(t)
	token := signTestToken(t, jwtManager, 42)

	createResponse := performVideoJSONRequest(
		router,
		http.MethodPost,
		"/api/videos",
		`{"title":"first video","description":"first description","media_url":"https://example.com/video.mp4","cover_url":"https://example.com/cover.jpg"}`,
		token,
		"create-video-1",
	)
	requireStatus(t, createResponse, http.StatusCreated)

	var created videoAPIResponse
	decodeJSON(t, createResponse, &created)
	if created.ID == 0 || created.AuthorID != 42 || created.Status != domainvideo.StatusPublished {
		t.Fatalf("unexpected create response: %+v", created)
	}
	if created.Title != "first video" || created.Description != "first description" || created.MediaURL == "" || created.CoverURL == "" || created.PublishedAt == nil {
		t.Fatalf("unexpected create response: %+v", created)
	}

	replayResponse := performVideoJSONRequest(
		router,
		http.MethodPost,
		"/api/videos",
		`{"title":"changed title","description":"changed description","media_url":"https://example.com/changed.mp4","cover_url":"https://example.com/changed.jpg"}`,
		token,
		"create-video-1",
	)
	requireStatus(t, replayResponse, http.StatusOK)

	var replayed videoAPIResponse
	decodeJSON(t, replayResponse, &replayed)
	if replayed.ID != created.ID || replayed.Title != created.Title || replayed.Description != created.Description {
		t.Fatalf("unexpected replay response: %+v", replayed)
	}

	getResponse := performVideoJSONRequest(router, http.MethodGet, fmt.Sprintf("/api/videos/%d", created.ID), "", "", "")
	requireStatus(t, getResponse, http.StatusOK)
	var fetched videoAPIResponse
	decodeJSON(t, getResponse, &fetched)
	if fetched.Description != created.Description {
		t.Fatalf("unexpected fetched response: %+v", fetched)
	}

	listResponse := performVideoJSONRequest(router, http.MethodGet, "/api/users/42/videos?limit=10&offset=0", "", "", "")
	requireStatus(t, listResponse, http.StatusOK)

	var list videoListAPIResponse
	decodeJSON(t, listResponse, &list)
	if len(list.Items) != 1 || list.Items[0].ID != created.ID || list.Items[0].Description != created.Description || list.Limit != 10 || list.Offset != 0 {
		t.Fatalf("unexpected author list response: %+v", list)
	}

	mineResponse := performVideoJSONRequest(router, http.MethodGet, "/api/videos/mine", "", token, "")
	requireStatus(t, mineResponse, http.StatusOK)

	var mine videoListAPIResponse
	decodeJSON(t, mineResponse, &mine)
	if len(mine.Items) != 1 || mine.Items[0].ID != created.ID || mine.Items[0].Description != created.Description {
		t.Fatalf("unexpected mine list response: %+v", mine)
	}

	deleteResponse := performVideoJSONRequest(router, http.MethodDelete, fmt.Sprintf("/api/videos/%d", created.ID), "", token, "")
	requireStatus(t, deleteResponse, http.StatusNoContent)

	getDeletedResponse := performVideoJSONRequest(router, http.MethodGet, fmt.Sprintf("/api/videos/%d", created.ID), "", "", "")
	requireStatus(t, getDeletedResponse, http.StatusNotFound)

	repeatDeleteResponse := performVideoJSONRequest(router, http.MethodDelete, fmt.Sprintf("/api/videos/%d", created.ID), "", token, "")
	requireStatus(t, repeatDeleteResponse, http.StatusNoContent)

	emptyListResponse := performVideoJSONRequest(router, http.MethodGet, "/api/users/42/videos", "", "", "")
	requireStatus(t, emptyListResponse, http.StatusOK)

	var emptyList videoListAPIResponse
	decodeJSON(t, emptyListResponse, &emptyList)
	if len(emptyList.Items) != 0 {
		t.Fatalf("unexpected empty list response: %+v", emptyList)
	}
}

// TestVideoAPIValidation 验证视频接口的认证、参数校验和资源权限响应。
func TestVideoAPIValidation(t *testing.T) {
	router, jwtManager := newVideoRouter(t)
	token := signTestToken(t, jwtManager, 42)

	unauthorizedCreateResponse := performVideoJSONRequest(
		router,
		http.MethodPost,
		"/api/videos",
		`{"title":"first video","media_url":"https://example.com/video.mp4","cover_url":"https://example.com/cover.jpg"}`,
		"",
		"",
	)
	requireStatus(t, unauthorizedCreateResponse, http.StatusUnauthorized)

	emptyTitleResponse := performVideoJSONRequest(
		router,
		http.MethodPost,
		"/api/videos",
		`{"title":"   ","media_url":"https://example.com/video.mp4","cover_url":"https://example.com/cover.jpg"}`,
		token,
		"",
	)
	requireStatus(t, emptyTitleResponse, http.StatusBadRequest)

	badIDResponse := performVideoJSONRequest(router, http.MethodGet, "/api/videos/abc", "", "", "")
	requireStatus(t, badIDResponse, http.StatusBadRequest)

	missingResponse := performVideoJSONRequest(router, http.MethodGet, "/api/videos/404", "", "", "")
	requireStatus(t, missingResponse, http.StatusNotFound)

	createResponse := performVideoJSONRequest(
		router,
		http.MethodPost,
		"/api/videos",
		`{"title":"owned video","media_url":"https://example.com/video.mp4","cover_url":"https://example.com/cover.jpg"}`,
		token,
		"",
	)
	requireStatus(t, createResponse, http.StatusCreated)

	var created videoAPIResponse
	decodeJSON(t, createResponse, &created)

	otherToken := signTestToken(t, jwtManager, 77)
	forbiddenDeleteResponse := performVideoJSONRequest(router, http.MethodDelete, fmt.Sprintf("/api/videos/%d", created.ID), "", otherToken, "")
	requireStatus(t, forbiddenDeleteResponse, http.StatusForbidden)

	badUserListResponse := performVideoJSONRequest(router, http.MethodGet, "/api/users/abc/videos", "", "", "")
	requireStatus(t, badUserListResponse, http.StatusBadRequest)

	badPaginationResponse := performVideoJSONRequest(router, http.MethodGet, "/api/users/42/videos?limit=0", "", "", "")
	requireStatus(t, badPaginationResponse, http.StatusBadRequest)
}

// newVideoRouter 构造使用内存仓储和测试 JWT 配置的视频路由。
func newVideoRouter(t *testing.T) (*gin.Engine, *authjwt.Manager) {
	router, manager, _ := newVideoRouterWithPublisher(t, nil)
	return router, manager
}

// newVideoRouterWithPublisher 注入发布器并创建使用真实认证中间件的视频测试路由。
func newVideoRouterWithPublisher(t *testing.T, publisher applicationvideo.PublishedEventPublisher) (*gin.Engine, *authjwt.Manager, *memoryVideoRepo) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()

	jwtManager, err := authjwt.NewManager(config.JWTConfig{
		Secret:     "video-api-test-secret-with-sufficient-length",
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("new JWT manager: %v", err)
	}

	repo := newMemoryVideoRepo()
	service := applicationvideo.NewService(repo, applicationvideo.WithPublishedEventPublisher(publisher))
	handler := videohandler.NewHandler(service)
	authMiddleware := middleware.NewJWTAuthMiddleware(jwtManager)

	api := router.Group("/api")
	videos := api.Group("/videos")
	videos.POST("", authMiddleware, handler.Create)
	videos.GET("/mine", authMiddleware, handler.ListMine)
	videos.GET("/:videoId", handler.Get)
	videos.DELETE("/:videoId", authMiddleware, handler.Delete)
	api.GET("/users/:userId/videos", handler.ListByAuthor)

	return router, jwtManager, repo
}

// signTestToken 为测试用户签发访问令牌。
func signTestToken(t *testing.T, jwtManager *authjwt.Manager, userID int64) string {
	t.Helper()

	token, err := jwtManager.SignAccessToken(userID, "user")
	if err != nil {
		t.Fatalf("sign test token: %v", err)
	}
	return token
}

// performVideoJSONRequest 向测试路由发送带 JSON、认证和幂等键的 HTTP 请求。
func performVideoJSONRequest(router *gin.Engine, method, path, body, accessToken, idempotencyKey string) *httptest.ResponseRecorder {
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

// requireStatus 断言 HTTP 响应状态码符合预期。
func requireStatus(t *testing.T, response *httptest.ResponseRecorder, want int) {
	t.Helper()
	if response.Code != want {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, want, response.Body.String())
	}
}

// decodeJSON 将 HTTP 响应体解析到指定结构中。
func decodeJSON(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response JSON: %v; body = %s", err, response.Body.String())
	}
}

// cloneVideo 复制视频及其可变时间指针，避免测试仓储暴露内部状态。
func cloneVideo(video *domainvideo.Video) *domainvideo.Video {
	cloned := *video
	if video.PublishedAt != nil {
		publishedAt := *video.PublishedAt
		cloned.PublishedAt = &publishedAt
	}
	return &cloned
}

// memoryVideoIdempotencyKey 生成按作者隔离的幂等键索引。
func memoryVideoIdempotencyKey(authorID int64, key string) string {
	return fmt.Sprintf("%d:%s", authorID, key)
}

// publishedAtUnix 返回视频发布时间的纳秒时间戳用于排序。
func publishedAtUnix(video *domainvideo.Video) int64 {
	if video.PublishedAt == nil {
		return 0
	}
	return video.PublishedAt.UnixNano()
}

var _ domainvideo.Repository = (*memoryVideoRepo)(nil)

// memoryVideoPublisher 以互斥锁保存测试发布事件。
type memoryVideoPublisher struct {
	mu     sync.Mutex
	events []*applicationvideo.PublishedEvent
}

// PublishVideoPublished 加锁记录测试发布事件。
func (p *memoryVideoPublisher) PublishVideoPublished(ctx context.Context, event *applicationvideo.PublishedEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, event)
	return nil
}

// EventCount 加锁读取测试发布事件总数。
func (p *memoryVideoPublisher) EventCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.events)
}

// TestVideoPublishedEvent 验证首次创建返回二百零一并发送事件，幂等重放返回二百且不重复发送。
func TestVideoPublishedEvent(t *testing.T) {
	publisher := &memoryVideoPublisher{}
	router, jwtManager, _ := newVideoRouterWithPublisher(t, publisher)
	token := signTestToken(t, jwtManager, 42)

	createResponse := performVideoJSONRequest(
		router,
		http.MethodPost,
		"/api/videos",
		`{"title":"event video","description":"event description","media_url":"https://example.com/video.mp4","cover_url":"https://example.com/cover.jpg"}`,
		token,
		"create-video-event",
	)
	requireStatus(t, createResponse, http.StatusCreated)
	if publisher.EventCount() != 1 {
		t.Fatalf("unexpected published event count after create: %d", publisher.EventCount())
	}

	replayResponse := performVideoJSONRequest(
		router,
		http.MethodPost,
		"/api/videos",
		`{"title":"changed","description":"changed","media_url":"https://example.com/changed.mp4","cover_url":"https://example.com/changed.jpg"}`,
		token,
		"create-video-event",
	)
	requireStatus(t, replayResponse, http.StatusOK)
	if publisher.EventCount() != 1 {
		t.Fatalf("unexpected published event count after replay: %d", publisher.EventCount())
	}
}

// failedVideoPublisher 模拟发送失败并记录调用次数。
type failedVideoPublisher struct{ calls int }

// PublishVideoPublished 返回模拟发送错误，验证已提交视频不会被回滚。
func (p *failedVideoPublisher) PublishVideoPublished(context.Context, *applicationvideo.PublishedEvent) error {
	p.calls++
	return errors.New("broker unavailable")
}

// TestPublisherFailureAndConcurrentIdempotency 验证发送失败不影响发布结果、重放不再发送、并发同键仅创建一个事件。
func TestPublisherFailureAndConcurrentIdempotency(t *testing.T) {
	failed := &failedVideoPublisher{}
	service := applicationvideo.NewService(newMemoryVideoRepo(), applicationvideo.WithPublishedEventPublisher(failed))
	for i := 0; i < 2; i++ {
		result, err := service.CreatePublished(context.Background(), 42, "title", "description", "media", "cover", "same")
		if err != nil || result.Created != (i == 0) {
			t.Fatalf("result=%+v err=%v", result, err)
		}
	}
	if failed.calls != 1 {
		t.Fatalf("calls=%d", failed.calls)
	}
	publisher := &memoryVideoPublisher{}
	service = applicationvideo.NewService(newMemoryVideoRepo(), applicationvideo.WithPublishedEventPublisher(publisher))
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := service.CreatePublished(context.Background(), 42, "title", "description", "media", "cover", "concurrent"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if publisher.EventCount() != 1 {
		t.Fatalf("events=%d", publisher.EventCount())
	}
}
