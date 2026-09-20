package feed

import (
	"context"
	"fmt"
	"github.com/inumoroha/video-feed/internal/config"
	"net/http"
	"sort"
	"sync"
	"testing"
	"time"

	applicationfeed "github.com/inumoroha/video-feed/internal/application/feed"
	applicationrecommendation "github.com/inumoroha/video-feed/internal/application/recommendation"
	infrajwt "github.com/inumoroha/video-feed/internal/auth/jwt"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	domainrecommendation "github.com/inumoroha/video-feed/internal/domain/recommendation"
	interfaceshttprecommendation "github.com/inumoroha/video-feed/internal/handler/recommendation"
	interfaceshttpmiddleware "github.com/inumoroha/video-feed/internal/middleware"

	"github.com/gin-gonic/gin"
)

// recommendationAPIResponse 接收推荐接口测试的分页响应。
type recommendationAPIResponse struct {
	UserID     int64                             `json:"user_id"`
	Scene      string                            `json:"scene"`
	RequestID  string                            `json:"request_id"`
	Candidates []recommendationCandidateResponse `json:"candidates"`
	NextCursor string                            `json:"next_cursor"`
	HasMore    bool                              `json:"has_more"`
}

// recommendationCandidateResponse 接收推荐候选测试字段。
type recommendationCandidateResponse struct {
	VideoID        int64     `json:"video_id"`
	AuthorID       int64     `json:"author_id"`
	RankScore      float64   `json:"rank_score"`
	Similarity     float64   `json:"similarity"`
	HotScore       int       `json:"hot_score"`
	FreshnessScore float64   `json:"freshness_score"`
	Reason         string    `json:"reason"`
	PublishedAt    time.Time `json:"published_at"`
}

// recommendationExposureAPIResponse 接收曝光接口测试的聚合列表。
type recommendationExposureAPIResponse struct {
	Exposures []recommendationExposureResponse `json:"exposures"`
}

// recommendationExposureResponse 接收曝光聚合测试字段。
type recommendationExposureResponse struct {
	UserID         int64     `json:"user_id"`
	VideoID        int64     `json:"video_id"`
	FirstExposedAt time.Time `json:"first_exposed_at"`
	LastExposedAt  time.Time `json:"last_exposed_at"`
	ExposureCount  int       `json:"exposure_count"`
	LastScene      string    `json:"last_scene"`
}

// memoryRecommendationRepo 通过互斥锁保护测试候选、向量和曝光聚合。
type memoryRecommendationRepo struct {
	mu        sync.Mutex
	pool      []*domainrecommendation.Candidate
	vectors   map[int64][]float64
	interest  map[int64][]float64
	published map[int64]bool
	exposures map[string]*domainrecommendation.Exposure
}

// newMemoryRecommendationRepo 初始化候选、兴趣向量及曝光测试数据。
func newMemoryRecommendationRepo() *memoryRecommendationRepo {
	base := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	return &memoryRecommendationRepo{
		pool: []*domainrecommendation.Candidate{
			domainrecommendation.RestoreCandidate(1, 10, 0, 0, 8, 0, "", base.Add(-4*time.Hour)),
			domainrecommendation.RestoreCandidate(2, 20, 0, 0, 40, 0, "", base.Add(-3*time.Hour)),
			domainrecommendation.RestoreCandidate(3, 30, 0, 0, 8, 0, "", base.Add(-2*time.Hour)),
			domainrecommendation.RestoreCandidate(4, 20, 0, 0, 6, 0, "", base.Add(-1*time.Hour)),
		},
		vectors: map[int64][]float64{
			1: {1, 0},
			2: {0, 1},
			3: {1, 0},
			4: {0.7, 0.3},
		},
		interest: map[int64][]float64{
			42: {1, 0},
		},
		published: map[int64]bool{1: true, 2: true, 3: true, 4: true},
		exposures: map[string]*domainrecommendation.Exposure{},
	}
}

// ListCandidatePool 加锁过滤测试曝光并按热度排序，返回候选副本。
func (r *memoryRecommendationRepo) ListCandidatePool(ctx context.Context, userID int64, limit int) ([]*domainrecommendation.Candidate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	items := make([]*domainrecommendation.Candidate, 0, len(r.pool))
	for _, item := range r.pool {
		if item == nil {
			continue
		}
		if r.exposures[recommendationExposureKey(userID, item.VideoID)] != nil {
			continue
		}
		value := *item
		items = append(items, &value)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].HotScore == items[j].HotScore {
			if items[i].PublishedAt.Equal(items[j].PublishedAt) {
				return items[i].VideoID > items[j].VideoID
			}
			return items[i].PublishedAt.After(items[j].PublishedAt)
		}
		return items[i].HotScore > items[j].HotScore
	})
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	return items, nil
}

// LoadUserInterestVector 加锁读取测试画像并返回向量副本。
func (r *memoryRecommendationRepo) LoadUserInterestVector(ctx context.Context, userID int64) ([]float64, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	vector := r.interest[userID]
	if len(vector) == 0 {
		return nil, false, nil
	}
	return append([]float64(nil), vector...), true, nil
}

// LoadVideoVectors 加锁读取指定测试视频向量并返回副本。
func (r *memoryRecommendationRepo) LoadVideoVectors(ctx context.Context, videoIDs []int64) (map[int64][]float64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	vectors := map[int64][]float64{}
	for _, videoID := range videoIDs {
		if vector := r.vectors[videoID]; len(vector) > 0 {
			vectors[videoID] = append([]float64(nil), vector...)
		}
	}
	return vectors, nil
}

// SaveExposures 加锁模拟曝光计数并复制返回值，未发布视频返回错误。
func (r *memoryRecommendationRepo) SaveExposures(ctx context.Context, writes []*domainrecommendation.ExposureWrite) ([]*domainrecommendation.Exposure, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Date(2026, 5, 3, 13, 0, 0, 0, time.UTC)
	exposures := make([]*domainrecommendation.Exposure, 0, len(writes))
	for _, write := range writes {
		if !r.published[write.VideoID] {
			return nil, domainrecommendation.ErrVideoNotFound
		}
		key := recommendationExposureKey(write.UserID, write.VideoID)
		exposure := r.exposures[key]
		if exposure == nil {
			exposure = domainrecommendation.RestoreExposure(int64(len(r.exposures)+1), write.UserID, write.VideoID, now, now, 1, write.Scene)
			r.exposures[key] = exposure
			exposures = append(exposures, cloneRecommendationExposure(exposure))
			continue
		}
		exposure.LastExposedAt = now.Add(time.Duration(exposure.ExposureCount) * time.Second)
		exposure.ExposureCount++
		exposure.LastScene = write.Scene
		exposures = append(exposures, cloneRecommendationExposure(exposure))
	}
	return exposures, nil
}

// TestRecommendationCandidatesAPI 验证兴趣排序、推荐原因、请求元数据及游标续页。
func TestRecommendationCandidatesAPI(t *testing.T) {
	router := newRecommendationRouter()

	response := performJSONRequest(
		router,
		http.MethodPost,
		"/internal/recommendation-candidates",
		`{"user_id":42,"scene":"recommend","request_id":"req-1","limit":2}`,
		"",
	)
	requireStatus(t, response, http.StatusOK)

	var page recommendationAPIResponse
	decodeJSON(t, response, &page)
	if page.UserID != 42 || page.Scene != "recommend" || page.RequestID != "req-1" {
		t.Fatalf("unexpected recommendation metadata: %+v", page)
	}
	if len(page.Candidates) != 2 || page.Candidates[0].VideoID != 3 || page.Candidates[1].VideoID != 1 {
		t.Fatalf("unexpected recommendation page: %+v", page)
	}
	if page.Candidates[0].Reason != "interest_match" || page.Candidates[0].Similarity <= 0 {
		t.Fatalf("unexpected recommendation reason: %+v", page.Candidates[0])
	}
	if page.NextCursor == "" || !page.HasMore {
		t.Fatalf("unexpected recommendation cursor: %+v", page)
	}

	nextResponse := performJSONRequest(
		router,
		http.MethodPost,
		"/internal/recommendation-candidates",
		fmt.Sprintf(`{"user_id":42,"scene":"recommend","cursor":%q,"limit":2}`, page.NextCursor),
		"",
	)
	requireStatus(t, nextResponse, http.StatusOK)

	var nextPage recommendationAPIResponse
	decodeJSON(t, nextResponse, &nextPage)
	if len(nextPage.Candidates) != 2 || nextPage.Candidates[0].VideoID != 4 || nextPage.Candidates[1].VideoID != 2 || nextPage.HasMore {
		t.Fatalf("unexpected recommendation next page: %+v", nextPage)
	}
}

// TestRecommendationExposuresAPI 验证批内去重、重复曝光计数、已曝光过滤及视频不存在响应。
func TestRecommendationExposuresAPI(t *testing.T) {
	router := newRecommendationRouter()

	firstResponse := performJSONRequest(
		router,
		http.MethodPost,
		"/internal/exposures",
		`{"user_id":42,"scene":"recommend","request_id":"req-exp","video_ids":[1,1,3]}`,
		"",
	)
	requireStatus(t, firstResponse, http.StatusCreated)

	var first recommendationExposureAPIResponse
	decodeJSON(t, firstResponse, &first)
	if len(first.Exposures) != 2 || first.Exposures[0].ExposureCount != 1 || first.Exposures[1].VideoID != 3 {
		t.Fatalf("unexpected exposure response: %+v", first)
	}

	secondResponse := performJSONRequest(
		router,
		http.MethodPost,
		"/internal/exposures",
		`{"user_id":42,"scene":"recommend","request_id":"req-exp-2","video_ids":[1]}`,
		"",
	)
	requireStatus(t, secondResponse, http.StatusCreated)

	var second recommendationExposureAPIResponse
	decodeJSON(t, secondResponse, &second)
	if len(second.Exposures) != 1 || second.Exposures[0].ExposureCount != 2 {
		t.Fatalf("unexpected repeated exposure response: %+v", second)
	}

	candidateResponse := performJSONRequest(
		router,
		http.MethodPost,
		"/internal/recommendation-candidates",
		`{"user_id":42,"scene":"recommend","limit":10}`,
		"",
	)
	requireStatus(t, candidateResponse, http.StatusOK)
	var page recommendationAPIResponse
	decodeJSON(t, candidateResponse, &page)
	for _, candidate := range page.Candidates {
		if candidate.VideoID == 1 || candidate.VideoID == 3 {
			t.Fatalf("exposed video returned: %+v", page)
		}
	}

	missingResponse := performJSONRequest(
		router,
		http.MethodPost,
		"/internal/exposures",
		`{"user_id":42,"scene":"recommend","video_ids":[404]}`,
		"",
	)
	requireStatus(t, missingResponse, http.StatusNotFound)
}

// TestRecommendFeedScene 验证推荐卡片排序、分页及匿名访问拒绝。
func TestRecommendFeedScene(t *testing.T) {
	recommendationRepo := newMemoryRecommendationRepo()
	recommendationService := applicationrecommendation.New(
		recommendationRepo,
		applicationrecommendation.WithNow(func() time.Time {
			return time.Date(2026, 5, 3, 13, 0, 0, 0, time.UTC)
		}),
	)
	feedRepo := newMemoryFeedRepo(seedRecommendFeedItems())
	router, jwtManager := newRecommendFeedRouterWithJWT(t, applicationfeed.New(feedRepo, applicationfeed.WithRecommender(recommendationService)))
	token := signTestToken(t, jwtManager, 42)

	response := performJSONRequest(router, http.MethodGet, "/api/feed-items?scene=recommend&limit=2", "", token)
	requireStatus(t, response, http.StatusOK)

	var page feedAPIResponse
	decodeJSON(t, response, &page)
	if page.Scene != string(domainfeed.SceneRecommend) {
		t.Fatalf("unexpected recommend feed scene: %+v", page)
	}
	if len(page.Items) != 2 || page.Items[0].VideoID != 3 || page.Items[1].VideoID != 1 || !page.HasMore {
		t.Fatalf("unexpected recommend feed response: %+v", page)
	}

	unauthorizedResponse := performJSONRequest(router, http.MethodGet, "/api/feed-items?scene=recommend&limit=2", "", "")
	requireStatus(t, unauthorizedResponse, http.StatusUnauthorized)
}

// newRecommendationRouter 创建使用内存仓储和固定时钟的推荐测试路由。
func newRecommendationRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	service := applicationrecommendation.New(
		newMemoryRecommendationRepo(),
		applicationrecommendation.WithNow(func() time.Time {
			return time.Date(2026, 5, 3, 13, 0, 0, 0, time.UTC)
		}),
	)
	handler := interfaceshttprecommendation.New(service)

	internal := router.Group("/internal")
	internal.POST("/recommendation-candidates", handler.ListCandidates)
	internal.POST("/exposure-decisions", handler.DecideExposures)
	internal.POST("/exposures", handler.SaveExposures)

	return router
}

// newRecommendFeedRouterWithJWT 使用本项目 JWT 配置创建推荐 Feed 测试路由。
func newRecommendFeedRouterWithJWT(t *testing.T, service *applicationfeed.Service) (*gin.Engine, *infrajwt.Manager) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	jwtManager, err := infrajwt.NewManager(config.JWTConfig{Secret: "recommendation-test-secret-2026-strong", AccessTTL: 15 * time.Minute, RefreshTTL: 24 * time.Hour})
	if err != nil {
		t.Fatalf("new jwt manager: %v", err)
	}
	handler := NewHandler(service)
	optionalAuthMiddleware := interfaceshttpmiddleware.NewOptionalJWTAuth(jwtManager)

	api := router.Group("/api")
	api.GET("/feed-items", optionalAuthMiddleware, handler.ListFeedItems)

	return router, jwtManager
}

// seedRecommendFeedItems 创建与推荐候选对应的测试视频卡片。
func seedRecommendFeedItems() []*domainfeed.FeedItem {
	base := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	return []*domainfeed.FeedItem{
		domainfeed.RestoreFeedItem(1, 10, "author 1", "https://example.com/a1.jpg", "match old", "match old description", "https://example.com/1.mp4", "https://example.com/1.jpg", 1, 1, 1, base.Add(-4*time.Hour)),
		domainfeed.RestoreFeedItem(2, 20, "author 2", "https://example.com/a2.jpg", "hot miss", "hot miss description", "https://example.com/2.mp4", "https://example.com/2.jpg", 4, 4, 4, base.Add(-3*time.Hour)),
		domainfeed.RestoreFeedItem(3, 30, "author 3", "https://example.com/a3.jpg", "match new", "match new description", "https://example.com/3.mp4", "https://example.com/3.jpg", 1, 1, 1, base.Add(-2*time.Hour)),
		domainfeed.RestoreFeedItem(4, 20, "author 2", "https://example.com/a2.jpg", "mixed", "mixed description", "https://example.com/4.mp4", "https://example.com/4.jpg", 1, 1, 0, base.Add(-1*time.Hour)),
	}
}

// cloneRecommendationExposure 复制测试曝光聚合，避免共享可变返回值。
func cloneRecommendationExposure(exposure *domainrecommendation.Exposure) *domainrecommendation.Exposure {
	cloned := *exposure
	return &cloned
}

// recommendationExposureKey 生成测试用户和视频的联合键。
func recommendationExposureKey(userID int64, videoID int64) string {
	return fmt.Sprintf("%d:%d", userID, videoID)
}

// TestRecommendationConcurrentExposures 验证同一请求标识在并发请求中继续计数，批内重复视频只写一次。
func TestRecommendationConcurrentExposures(t *testing.T) {
	repo := newMemoryRecommendationRepo()
	service := applicationrecommendation.New(repo)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.SaveExposures(context.Background(), []applicationrecommendation.ExposureInput{{UserID: 42, VideoID: 1, Scene: "recommend", RequestID: "same"}, {UserID: 42, VideoID: 1, Scene: "recommend", RequestID: "same"}})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.exposures[recommendationExposureKey(42, 1)].ExposureCount != 32 {
		t.Fatal(repo.exposures)
	}
}

// TestRecommendationRequestErrors 验证内部接口的绑定、校验、空批次和页大小默认行为。
func TestRecommendationRequestErrors(t *testing.T) {
	router := newRecommendationRouter()
	for _, tc := range []struct {
		path, body string
		status     int
	}{
		{"/internal/recommendation-candidates", `{`, 400},
		{"/internal/recommendation-candidates", `{"user_id":0,"scene":"recommend"}`, 400},
		{"/internal/recommendation-candidates", `{"user_id":42}`, 400},
		{"/internal/recommendation-candidates", `{"user_id":42,"scene":"recommend","cursor":"!"}`, 400},
		{"/internal/recommendation-candidates", `{"user_id":42,"scene":"recommend","limit":-1}`, 200},
		{"/internal/exposures", `{`, 400},
		{"/internal/exposures", `{"user_id":42,"scene":"recommend","video_ids":[0]}`, 400},
		{"/internal/exposures", `{}`, 201},
	} {
		response := performJSONRequest(router, http.MethodPost, tc.path, tc.body, "")
		requireStatus(t, response, tc.status)
	}
}
