package feed

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	applicationfeed "github.com/inumoroha/video-feed/internal/application/feed"
	infrajwt "github.com/inumoroha/video-feed/internal/auth/jwt"
	"github.com/inumoroha/video-feed/internal/config"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	interfaceshttpmiddleware "github.com/inumoroha/video-feed/internal/middleware"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// feedAPIResponse 保存测试接口的分页响应。
type feedAPIResponse struct {
	Scene      string                `json:"scene"`
	Items      []feedItemAPIResponse `json:"items"`
	NextCursor string                `json:"next_cursor"`
	HasMore    bool                  `json:"has_more"`
}

// feedItemAPIResponse 保存测试接口的视频卡片字段。
type feedItemAPIResponse struct {
	VideoID         int64     `json:"video_id"`
	AuthorID        int64     `json:"author_id"`
	AuthorNickname  string    `json:"author_nickname"`
	AuthorAvatarURL string    `json:"author_avatar_url"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	MediaURL        string    `json:"media_url"`
	CoverURL        string    `json:"cover_url"`
	LikeCount       int       `json:"like_count"`
	CommentCount    int       `json:"comment_count"`
	FavoriteCount   int       `json:"favorite_count"`
	PublishedAt     time.Time `json:"published_at"`
}

// memoryFeedRepo 以互斥锁保护测试视频、关注关系和收件箱状态。
type memoryFeedRepo struct {
	mu                       sync.Mutex
	items                    []*domainfeed.FeedItem
	timelineCalls            int
	hotCalls                 int
	cardCalls                int
	statCalls                int
	following                map[int64]map[int64]struct{}
	followerCounts           map[int64]int
	inbox                    map[int64]map[int64]struct{}
	followingCalls           int
	followingPullAuthorCalls int
}

// newMemoryFeedRepo 初始化测试视频、关注关系、粉丝数和收件箱。
func newMemoryFeedRepo(items []*domainfeed.FeedItem) *memoryFeedRepo {
	return &memoryFeedRepo{
		items:          items,
		following:      map[int64]map[int64]struct{}{},
		followerCounts: map[int64]int{},
		inbox:          map[int64]map[int64]struct{}{},
	}
}

// FollowingCalls 加锁读取关注流仓储调用次数。
func (r *memoryFeedRepo) FollowingCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.followingCalls
}

// FollowForTest 加锁添加测试用户的关注关系。
func (r *memoryFeedRepo) FollowForTest(viewerID int64, authorID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.following[viewerID] == nil {
		r.following[viewerID] = map[int64]struct{}{}
	}
	r.following[viewerID][authorID] = struct{}{}
}

// SetFollowerCountForTest 加锁设置测试作者粉丝数。
func (r *memoryFeedRepo) SetFollowerCountForTest(authorID int64, followerCount int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.followerCounts[authorID] = followerCount
}

// PushToInboxForTest 加锁写入测试用户收件箱。
func (r *memoryFeedRepo) PushToInboxForTest(viewerID int64, videoID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.inbox[viewerID] == nil {
		r.inbox[viewerID] = map[int64]struct{}{}
	}
	r.inbox[viewerID][videoID] = struct{}{}
}

// ListTimelinePage 加锁模拟时间线排序与游标分页。
func (r *memoryFeedRepo) ListTimelinePage(ctx context.Context, cursor *domainfeed.TimelineCursor, limit int) ([]*domainfeed.FeedPageItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.timelineCalls++

	items := make([]*domainfeed.FeedPageItem, 0, len(r.items))
	for _, item := range r.items {
		// cursor 代表上一页最后一条数据，下一页从它之后开始。
		if cursor == nil || item.PublishedAt.Before(cursor.PublishedAt) || (item.PublishedAt.Equal(cursor.PublishedAt) && item.VideoID < cursor.VideoID) {
			items = append(items, feedPageItemFromFeedItem(item))
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].PublishedAt.Equal(items[j].PublishedAt) {
			return items[i].VideoID > items[j].VideoID
		}
		return items[i].PublishedAt.After(items[j].PublishedAt)
	})

	if limit > len(items) {
		limit = len(items)
	}
	return items[:limit], nil
}

// ListHotPage 加锁模拟热榜排序与游标分页。
func (r *memoryFeedRepo) ListHotPage(ctx context.Context, cursor *domainfeed.HotCursor, limit int) ([]*domainfeed.FeedPageItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hotCalls++

	items := make([]*domainfeed.FeedPageItem, 0, len(r.items))
	for _, item := range r.items {
		if cursor == nil ||
			item.HotScore < cursor.HotScore ||
			(item.HotScore == cursor.HotScore && item.PublishedAt.Before(cursor.PublishedAt)) ||
			(item.HotScore == cursor.HotScore && item.PublishedAt.Equal(cursor.PublishedAt) && item.VideoID < cursor.VideoID) {
			items = append(items, feedPageItemFromFeedItem(item))
		}
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

	if limit > len(items) {
		limit = len(items)
	}
	return items[:limit], nil
}

// ListFollowingPage 加锁读取有效关注作者的全部视频，并去重分页。
func (r *memoryFeedRepo) ListFollowingPage(ctx context.Context, viewerID int64, cursor *domainfeed.TimelineCursor, limit int) ([]*domainfeed.FeedPageItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.followingCalls++

	followedAuthors := r.following[viewerID]
	seen := map[int64]struct{}{}
	items := make([]*domainfeed.FeedPageItem, 0, len(r.items))
	for _, item := range r.items {
		_, followed := followedAuthors[item.AuthorID]
		if !followed {
			continue
		}
		if cursor == nil || item.PublishedAt.Before(cursor.PublishedAt) || (item.PublishedAt.Equal(cursor.PublishedAt) && item.VideoID < cursor.VideoID) {
			if _, ok := seen[item.VideoID]; ok {
				continue
			}
			seen[item.VideoID] = struct{}{}
			items = append(items, feedPageItemFromFeedItem(item))
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].PublishedAt.Equal(items[j].PublishedAt) {
			return items[i].VideoID > items[j].VideoID
		}
		return items[i].PublishedAt.After(items[j].PublishedAt)
	})

	if limit > len(items) {
		limit = len(items)
	}
	return items[:limit], nil
}

// BatchGetFeedCards 加锁读取测试视频卡片。
func (r *memoryFeedRepo) BatchGetFeedCards(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedCard, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cardCalls++

	cards := map[int64]*domainfeed.FeedCard{}
	wanted := int64Set(videoIDs)
	for _, item := range r.items {
		if _, ok := wanted[item.VideoID]; ok {
			cards[item.VideoID] = feedCardFromFeedItem(item)
		}
	}
	return cards, nil
}

// BatchGetFeedStats 加锁读取测试互动统计。
func (r *memoryFeedRepo) BatchGetFeedStats(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statCalls++

	stats := map[int64]*domainfeed.FeedStat{}
	wanted := int64Set(videoIDs)
	for _, item := range r.items {
		if _, ok := wanted[item.VideoID]; ok {
			stats[item.VideoID] = feedStatFromFeedItem(item)
		}
	}
	return stats, nil
}

// TestFollowingFeedScene 验证数据库回退读取、未关注过滤和两页游标结果。
func TestFollowingFeedScene(t *testing.T) {
	repo := newMemoryFeedRepo(seedFollowingFeedItems())
	repo.FollowForTest(42, 100)
	repo.FollowForTest(42, 200)
	repo.SetFollowerCountForTest(100, domainfeed.BigCreatorFollowerThreshold)
	repo.SetFollowerCountForTest(200, domainfeed.BigCreatorFollowerThreshold-1)
	repo.SetFollowerCountForTest(300, domainfeed.BigCreatorFollowerThreshold)
	repo.PushToInboxForTest(42, 2)
	router, jwtManager := newFeedRouterWithServiceAndJWT(t, applicationfeed.New(repo))
	token := signTestToken(t, jwtManager, 42)

	firstPageResponse := performJSONRequest(router, http.MethodGet, "/api/feed-items?scene=following&limit=2", "", token)
	requireStatus(t, firstPageResponse, http.StatusOK)

	var firstPage feedAPIResponse
	decodeJSON(t, firstPageResponse, &firstPage)
	if firstPage.Scene != string(domainfeed.SceneFollowing) {
		t.Fatalf("unexpected following feed scene: %+v", firstPage)
	}
	if len(firstPage.Items) != 2 || firstPage.Items[0].VideoID != 4 || firstPage.Items[1].VideoID != 2 || !firstPage.HasMore {
		t.Fatalf("unexpected following first page response: %+v", firstPage)
	}
	if firstPage.NextCursor == "" {
		t.Fatalf("expected following cursor")
	}

	secondPageResponse := performJSONRequest(router, http.MethodGet, "/api/feed-items?scene=following&limit=2&cursor="+firstPage.NextCursor, "", token)
	requireStatus(t, secondPageResponse, http.StatusOK)

	var secondPage feedAPIResponse
	decodeJSON(t, secondPageResponse, &secondPage)
	if len(secondPage.Items) != 1 || secondPage.Items[0].VideoID != 1 || secondPage.HasMore {
		t.Fatalf("unexpected following second page response: %+v", secondPage)
	}
	if repo.FollowingCalls() != 2 {
		t.Fatalf("unexpected following repo calls: %d", repo.FollowingCalls())
	}
}

// TestFeedSceneQuery 验证时间线参数、请求体查询和匿名关注流的未认证响应。
func TestFeedSceneQuery(t *testing.T) {
	router := newFeedRouter(seedFeedItems())

	sceneResponse := performJSONRequest(router, http.MethodGet, "/api/feed-items?scene=timeline&limit=1", "", "")
	requireStatus(t, sceneResponse, http.StatusOK)

	var sceneFeed feedAPIResponse
	decodeJSON(t, sceneResponse, &sceneFeed)
	if sceneFeed.Scene != string(domainfeed.SceneTimeline) || len(sceneFeed.Items) != 1 || sceneFeed.Items[0].VideoID != 3 {
		t.Fatalf("unexpected scene response: %+v", sceneFeed)
	}

	queryResponse := performJSONRequest(
		router,
		http.MethodPost,
		"/api/feed/query",
		`{"scene":"timeline","limit":2,"context":{"device":"ios","experiment":"rank_v1"}}`,
		"",
	)
	requireStatus(t, queryResponse, http.StatusOK)

	var queryFeed feedAPIResponse
	decodeJSON(t, queryResponse, &queryFeed)
	if queryFeed.Scene != string(domainfeed.SceneTimeline) || len(queryFeed.Items) != 2 {
		t.Fatalf("unexpected query response: %+v", queryFeed)
	}

	followingResponse := performJSONRequest(router, http.MethodGet, "/api/feed-items?scene=following&limit=1", "", "")
	requireStatus(t, followingResponse, http.StatusUnauthorized)
}

// newFeedRouterWithServiceAndJWT 装配使用本项目可选认证中间件的测试路由和令牌管理器。
func newFeedRouterWithServiceAndJWT(t *testing.T, service *applicationfeed.Service) (*gin.Engine, *infrajwt.Manager) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	jwtManager, err := infrajwt.NewManager(config.JWTConfig{Secret: "following-test-secret-at-least-32-bytes", AccessTTL: 15 * time.Minute, RefreshTTL: 24 * time.Hour})
	if err != nil {
		t.Fatalf("new jwt manager: %v", err)
	}

	handler := NewHandler(service)
	optionalAuthMiddleware := interfaceshttpmiddleware.NewOptionalJWTAuth(jwtManager)

	api := router.Group("/api")
	api.GET("/feed-items", optionalAuthMiddleware, handler.ListFeedItems)
	api.POST("/feed/query", optionalAuthMiddleware, handler.Query)

	return router, jwtManager
}

// TestFollowingAuthenticationAndQuery 验证两种查询入口的认证、空页、非法游标和业务错误映射。
func TestFollowingAuthenticationAndQuery(t *testing.T) {
	repo := newMemoryFeedRepo(seedFollowingFeedItems())
	router, manager := newFeedRouterWithServiceAndJWT(t, applicationfeed.NewService(repo))
	token := signTestToken(t, manager, 42)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		target, body := "/api/feed-items?scene=following", ""
		if method == http.MethodPost {
			target, body = "/api/feed/query", `{"scene":"following"}`
		}
		for _, invalidToken := range []string{"", "invalid-token"} {
			w := performJSONRequest(router, method, target, body, invalidToken)
			requireStatus(t, w, http.StatusUnauthorized)
			if !strings.Contains(w.Body.String(), `"error":"viewer is required"`) {
				t.Fatal(w.Body.String())
			}
		}
		w := performJSONRequest(router, method, target, body, token)
		requireStatus(t, w, http.StatusOK)
		var page feedAPIResponse
		decodeJSON(t, w, &page)
		if page.Scene != "following" || len(page.Items) != 0 || page.HasMore || page.NextCursor != "" {
			t.Fatalf("%+v", page)
		}
	}
	requireStatus(t, performJSONRequest(router, http.MethodGet, "/api/feed-items?scene=following&cursor=bad", "", token), http.StatusBadRequest)
	requireStatus(t, performJSONRequest(router, http.MethodPost, "/api/feed/query", `{"scene":"following","limit":0}`, token), http.StatusBadRequest)
	requireStatus(t, performJSONRequest(router, http.MethodPost, "/api/feed/query", `{`, token), http.StatusBadRequest)
	requireStatus(t, performJSONRequest(router, http.MethodGet, "/api/feed-items?scene=unknown", "", token), http.StatusBadRequest)
	failingRouter, _ := newFeedRouterWithServiceAndJWT(t, applicationfeed.NewService(&handlerTestRepository{err: errors.New("database unavailable")}))
	requireStatus(t, performJSONRequest(failingRouter, http.MethodGet, "/api/feed-items?scene=following", "", token), http.StatusInternalServerError)
}

// TestFollowingDeduplicationAndUnfollow 验证重复条目去重、同时间排序、普通作者冷启动回源及取关过滤。
func TestFollowingDeduplicationAndUnfollow(t *testing.T) {
	items := seedFollowingFeedItems()
	items[0].PublishedAt = items[3].PublishedAt
	repo := newMemoryFeedRepo(items)
	repo.FollowForTest(42, 100)
	repo.FollowForTest(42, 200)
	repo.SetFollowerCountForTest(100, domainfeed.BigCreatorFollowerThreshold)
	repo.PushToInboxForTest(42, 4)
	repo.PushToInboxForTest(42, 3)
	router, manager := newFeedRouterWithServiceAndJWT(t, applicationfeed.NewService(repo))
	token := signTestToken(t, manager, 42)
	w := performJSONRequest(router, http.MethodPost, "/api/feed/query", `{"scene":"following","limit":1}`, token)
	requireStatus(t, w, http.StatusOK)
	var first feedAPIResponse
	decodeJSON(t, w, &first)
	if len(first.Items) != 1 || first.Items[0].VideoID != 4 || !first.HasMore {
		t.Fatalf("%+v", first)
	}
	w = performJSONRequest(router, http.MethodGet, "/api/feed-items?scene=following&limit=1&cursor="+first.NextCursor, "", token)
	requireStatus(t, w, http.StatusOK)
	var second feedAPIResponse
	decodeJSON(t, w, &second)
	if len(second.Items) != 1 || second.Items[0].VideoID != 1 || !second.HasMore {
		t.Fatalf("%+v", second)
	}
	repo.mu.Lock()
	delete(repo.following[42], 100)
	repo.mu.Unlock()
	w = performJSONRequest(router, http.MethodGet, "/api/feed-items?scene=following", "", token)
	requireStatus(t, w, http.StatusOK)
	decodeJSON(t, w, &second)
	if len(second.Items) != 1 || second.Items[0].VideoID != 2 {
		t.Fatalf("unfollowed entries leaked: %+v", second)
	}
}

// seedFollowingFeedItems 构造大作者、普通作者和未关注作者的四条测试视频。
func seedFollowingFeedItems() []*domainfeed.FeedItem {
	base := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	return []*domainfeed.FeedItem{
		domainfeed.RestoreFeedItem(1, 100, "big creator", "https://example.com/a100.jpg", "big old", "big old description", "https://example.com/1.mp4", "https://example.com/1.jpg", 1, 0, 0, base.Add(-3*time.Hour)),
		domainfeed.RestoreFeedItem(2, 200, "small creator", "https://example.com/a200.jpg", "small middle", "small middle description", "https://example.com/2.mp4", "https://example.com/2.jpg", 2, 0, 0, base.Add(-2*time.Hour)),
		domainfeed.RestoreFeedItem(3, 300, "unfollowed creator", "https://example.com/a300.jpg", "unfollowed", "unfollowed description", "https://example.com/3.mp4", "https://example.com/3.jpg", 3, 0, 0, base.Add(-1*time.Hour)),
		domainfeed.RestoreFeedItem(4, 100, "big creator", "https://example.com/a100.jpg", "big new", "big new description", "https://example.com/4.mp4", "https://example.com/4.jpg", 4, 0, 0, base),
	}
}

// feedPageItemFromFeedItem 将完整测试视频转换为轻量页项。
func feedPageItemFromFeedItem(item *domainfeed.FeedItem) *domainfeed.FeedPageItem {
	return &domainfeed.FeedPageItem{
		VideoID:     item.VideoID,
		AuthorID:    item.AuthorID,
		PublishedAt: item.PublishedAt,
		HotScore:    item.HotScore,
	}
}

// seedFeedItems 构造时间线查询的三条测试视频。
func seedFeedItems() []*domainfeed.FeedItem {
	base := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	return []*domainfeed.FeedItem{
		domainfeed.RestoreFeedItem(1, 10, "old author", "https://example.com/avatar-1.jpg", "old video", "old description", "https://example.com/1.mp4", "https://example.com/1.jpg", 1, 2, 3, base.Add(-2*time.Hour)),
		domainfeed.RestoreFeedItem(2, 20, "middle author", "https://example.com/avatar-2.jpg", "middle video", "middle description", "https://example.com/2.mp4", "https://example.com/2.jpg", 4, 5, 6, base.Add(-1*time.Hour)),
		domainfeed.RestoreFeedItem(3, 30, "new author", "https://example.com/avatar-3.jpg", "new video", "new description", "https://example.com/3.mp4", "https://example.com/3.jpg", 7, 8, 9, base),
	}
}

// newFeedRouterWithService 装配不带认证的测试信息流路由。
func newFeedRouterWithService(service *applicationfeed.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	handler := NewHandler(service)

	api := router.Group("/api")
	api.GET("/feed-items", handler.ListFeedItems)
	api.POST("/feed/query", handler.Query)

	return router
}

// performJSONRequest 执行带可选令牌的 JSON 请求并返回记录器。
func performJSONRequest(router http.Handler, method, target, body, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

// requireStatus 校验 HTTP 状态并在失败时输出响应体。
func requireStatus(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status=%d want=%d body=%s", w.Code, status, w.Body.String())
	}
}

// decodeJSON 将测试响应解码到指定结构。
func decodeJSON(t *testing.T, w *httptest.ResponseRecorder, result any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), result); err != nil {
		t.Fatal(err)
	}
}

// signTestToken 为测试用户签发本项目的访问令牌。
func signTestToken(t *testing.T, manager *infrajwt.Manager, userID int64) string {
	t.Helper()
	token, err := manager.SignAccessToken(userID, "user")
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// int64Set 将视频编号列表转换为集合。
func int64Set(values []int64) map[int64]struct{} {
	set := map[int64]struct{}{}
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

// feedCardFromFeedItem 提取测试视频卡片字段。
func feedCardFromFeedItem(item *domainfeed.FeedItem) *domainfeed.FeedCard {
	return &domainfeed.FeedCard{
		VideoID:         item.VideoID,
		AuthorID:        item.AuthorID,
		AuthorNickname:  item.AuthorNickname,
		AuthorAvatarURL: item.AuthorAvatarURL,
		Title:           item.Title,
		Description:     item.Description,
		MediaURL:        item.MediaURL,
		CoverURL:        item.CoverURL,
	}
}

// feedStatFromFeedItem 提取测试视频统计字段。
func feedStatFromFeedItem(item *domainfeed.FeedItem) *domainfeed.FeedStat {
	return &domainfeed.FeedStat{
		VideoID:       item.VideoID,
		LikeCount:     item.LikeCount,
		CommentCount:  item.CommentCount,
		FavoriteCount: item.FavoriteCount,
	}
}

// newFeedRouter 以测试视频创建信息流服务和路由。
func newFeedRouter(items []*domainfeed.FeedItem) *gin.Engine {
	return newFeedRouterWithService(applicationfeed.New(newMemoryFeedRepo(items)))
}

// ListFollowingPullAuthorIDs 查询当前用户关注的大作者编号。
func (r *memoryFeedRepo) ListFollowingPullAuthorIDs(ctx context.Context, viewerID int64) ([]int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.followingPullAuthorCalls++

	authors := make([]int64, 0, len(r.following[viewerID]))
	for authorID := range r.following[viewerID] {
		if r.followerCounts[authorID] < domainfeed.BigCreatorFollowerThreshold {
			continue
		}
		authors = append(authors, authorID)
	}
	sort.Slice(authors, func(i, j int) bool {
		return authors[i] < authors[j]
	})
	return authors, nil
}

// FollowingPullAuthorCalls 加锁读取索引作者查询次数。
func (r *memoryFeedRepo) FollowingPullAuthorCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.followingPullAuthorCalls
}

// ListFollowingAuthorIDs 返回所有有效关注作者，并记录索引读取的作者查询次数。
func (r *memoryFeedRepo) ListFollowingAuthorIDs(_ context.Context, viewerID int64) ([]int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.followingPullAuthorCalls++
	ids := []int64{}
	for id := range r.following[viewerID] {
		ids = append(ids, id)
	}
	return ids, nil
}
