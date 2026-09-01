package feed

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	applicationfeed "github.com/inumoroha/video-feed/internal/application/feed"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
)

// feedAPIResponse 表示信息流 API 测试中解析的分页响应。
type feedAPIResponse struct {
	Items      []feedItemAPIResponse `json:"items"`
	NextCursor string                `json:"next_cursor"`
	HasMore    bool                  `json:"has_more"`
}

// feedItemAPIResponse 表示信息流 API 测试中解析的视频条目。
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

// memoryFeedRepo 提供信息流 API 流程测试使用的并发安全内存仓储。
type memoryFeedRepo struct {
	mu    sync.Mutex
	items []*domainfeed.FeedItem
}

// newMemoryFeedRepo 创建用于信息流 HTTP 流程测试的内存仓储。
func newMemoryFeedRepo(items []*domainfeed.FeedItem) *memoryFeedRepo {
	return &memoryFeedRepo{items: items}
}

// ListTimelineFeed 按时间和视频 ID 倒序返回内存信息流，并应用游标过滤。
func (r *memoryFeedRepo) ListTimelineFeed(_ context.Context, cursor *domainfeed.TimelineCursor, limit int) ([]*domainfeed.FeedItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	items := make([]*domainfeed.FeedItem, 0, len(r.items))
	for _, item := range r.items {
		if cursor == nil || item.PublishedAt.Before(cursor.PublishedAt) || (item.PublishedAt.Equal(cursor.PublishedAt) && item.VideoID < cursor.VideoID) {
			items = append(items, cloneFeedItem(item))
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

// TestFeedAPIFlow 验证信息流分页、刷新和响应字段的完整 HTTP 流程。
func TestFeedAPIFlow(t *testing.T) {
	router := newFeedRouter(seedFeedItems())

	firstPageResponse := performFeedJSONRequest(router, http.MethodGet, "/api/feed/time?limit=2", "")
	requireStatus(t, firstPageResponse, http.StatusOK)

	var firstPage feedAPIResponse
	decodeJSON(t, firstPageResponse, &firstPage)
	if len(firstPage.Items) != 2 || firstPage.Items[0].VideoID != 3 || firstPage.Items[0].Description != "new description" || firstPage.Items[0].AuthorNickname != "new author" || firstPage.Items[1].VideoID != 2 {
		t.Fatalf("unexpected first page response: %+v", firstPage)
	}
	if firstPage.NextCursor == "" || !firstPage.HasMore {
		t.Fatalf("unexpected first page cursor: %+v", firstPage)
	}

	secondPageResponse := performFeedJSONRequest(router, http.MethodGet, "/api/feed/time?cursor="+firstPage.NextCursor+"&limit=2", "")
	requireStatus(t, secondPageResponse, http.StatusOK)

	var secondPage feedAPIResponse
	decodeJSON(t, secondPageResponse, &secondPage)
	if len(secondPage.Items) != 1 || secondPage.Items[0].VideoID != 1 || secondPage.HasMore || secondPage.NextCursor != "" {
		t.Fatalf("unexpected second page response: %+v", secondPage)
	}

	refreshResponse := performFeedJSONRequest(router, http.MethodGet, "/api/feed/refresh?limit=1", "")
	requireStatus(t, refreshResponse, http.StatusOK)

	var refresh feedAPIResponse
	decodeJSON(t, refreshResponse, &refresh)
	if len(refresh.Items) != 1 || refresh.Items[0].VideoID != 3 || !refresh.HasMore {
		t.Fatalf("unexpected refresh response: %+v", refresh)
	}
}

// TestFeedAPIValidation 验证信息流查询接口的参数校验响应。
func TestFeedAPIValidation(t *testing.T) {
	router := newFeedRouter(seedFeedItems())

	badLimitResponse := performFeedJSONRequest(router, http.MethodGet, "/api/feed/time?limit=0", "")
	requireStatus(t, badLimitResponse, http.StatusBadRequest)

	badCursorResponse := performFeedJSONRequest(router, http.MethodGet, "/api/feed/time?cursor=bad-cursor", "")
	requireStatus(t, badCursorResponse, http.StatusBadRequest)
}

// newFeedRouter 使用内存仓储组装信息流测试路由。
func newFeedRouter(items []*domainfeed.FeedItem) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	service := applicationfeed.NewService(newMemoryFeedRepo(items))
	handler := NewHandler(service)

	api := router.Group("/api")
	feed := api.Group("/feed")
	feed.GET("/time", handler.Timeline)
	feed.GET("/refresh", handler.Refresh)

	return router
}

// performFeedJSONRequest 向信息流测试路由发送 HTTP 请求。
func performFeedJSONRequest(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

// seedFeedItems 创建按发布时间递减排列的信息流测试数据。
func seedFeedItems() []*domainfeed.FeedItem {
	base := time.Date(2026, time.May, 3, 12, 0, 0, 0, time.UTC)
	return []*domainfeed.FeedItem{
		domainfeed.RestoreFeedItem(1, 10, "old author", "old avatar", "old video", "old description", "https://example.com/1.mp4", "https://example.com/1.jpg", 1, 2, 3, base.Add(-2*time.Hour)),
		domainfeed.RestoreFeedItem(2, 20, "middle author", "middle avatar", "middle video", "middle description", "https://example.com/2.mp4", "https://example.com/2.jpg", 4, 5, 6, base.Add(-1*time.Hour)),
		domainfeed.RestoreFeedItem(3, 30, "new author", "new avatar", "new video", "new description", "https://example.com/3.mp4", "https://example.com/3.jpg", 7, 8, 9, base),
	}
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

// cloneFeedItem 复制信息流条目，避免测试仓储暴露内部状态。
func cloneFeedItem(item *domainfeed.FeedItem) *domainfeed.FeedItem {
	cloned := *item
	return &cloned
}

var _ domainfeed.Repository = (*memoryFeedRepo)(nil)
