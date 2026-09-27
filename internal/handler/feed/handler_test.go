package feed

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	applicationfeed "github.com/inumoroha/video-feed/internal/application/feed"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
)

// handlerTestRepository 提供 Handler 测试所需的时间线仓储实现。
type handlerTestRepository struct {
	items []*domainfeed.FeedItem
	err   error
}

// ListTimelinePage 返回测试时间线分页结果。
func (r *handlerTestRepository) ListTimelinePage(_ context.Context, _ *domainfeed.TimelineCursor, _ int) ([]*domainfeed.FeedPageItem, error) {
	out := make([]*domainfeed.FeedPageItem, 0, len(r.items))
	for _, i := range r.items {
		out = append(out, &domainfeed.FeedPageItem{VideoID: i.VideoID, PublishedAt: i.PublishedAt, HotScore: i.HotScore})
	}
	return out, r.err
}

// ListHotPage 返回测试热榜分页结果。
func (r *handlerTestRepository) ListHotPage(_ context.Context, _ *domainfeed.HotCursor, _ int) ([]*domainfeed.FeedPageItem, error) {
	return nil, r.err
}

// BatchGetFeedCards 返回指定视频的卡片数据，供测试组装或预热。
func (r *handlerTestRepository) BatchGetFeedCards(_ context.Context, ids []int64) (map[int64]*domainfeed.FeedCard, error) {
	m := map[int64]*domainfeed.FeedCard{}
	for _, id := range ids {
		for _, i := range r.items {
			if i.VideoID == id {
				m[id] = &domainfeed.FeedCard{VideoID: id, AuthorID: i.AuthorID, AuthorNickname: i.AuthorNickname, AuthorAvatarURL: i.AuthorAvatarURL, Title: i.Title, Description: i.Description, MediaURL: i.MediaURL, CoverURL: i.CoverURL}
			}
		}
	}
	return m, r.err
}

// BatchGetFeedStats 返回指定视频的互动统计，供测试组装或预热。
func (r *handlerTestRepository) BatchGetFeedStats(_ context.Context, ids []int64) (map[int64]*domainfeed.FeedStat, error) {
	m := map[int64]*domainfeed.FeedStat{}
	for _, id := range ids {
		for _, i := range r.items {
			if i.VideoID == id {
				m[id] = &domainfeed.FeedStat{VideoID: id, LikeCount: i.LikeCount, CommentCount: i.CommentCount, FavoriteCount: i.FavoriteCount}
			}
		}
	}
	return m, r.err
}

// BatchGetViewerActionStates 返回空的查看者互动状态，供不校验互动状态的用例满足仓储接口。
func (r *handlerTestRepository) BatchGetViewerActionStates(context.Context, int64, []int64) (map[int64]*domainfeed.ViewerActionState, error) {
	return map[int64]*domainfeed.ViewerActionState{}, r.err
}

// ListTimelineFeed 返回测试信息流或预设错误。
func (r *handlerTestRepository) ListTimelineFeed(_ context.Context, _ *domainfeed.TimelineCursor, _ int) ([]*domainfeed.FeedItem, error) {
	return r.items, r.err
}

// ListHotFeed 返回测试热榜信息流或预设错误。
func (r *handlerTestRepository) ListHotFeed(_ context.Context, _ *domainfeed.HotCursor, _ int) ([]*domainfeed.FeedItem, error) {
	return r.items, r.err
}

// TestTimelineWithNilServiceReturnsInternalServerError 验证信息流服务未初始化时返回服务器错误。
func TestTimelineWithNilServiceReturnsInternalServerError(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx := newFeedTestContext(http.MethodGet, "/api/feed/time", "", recorder)

	NewHandler(nil).Timeline(ctx)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("Timeline() status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

// TestTimelineReturnsMappedResponse 验证时间线接口返回视频和作者字段。
func TestTimelineReturnsMappedResponse(t *testing.T) {
	publishedAt := time.Date(2026, time.August, 31, 12, 0, 0, 0, time.UTC)
	repository := &handlerTestRepository{items: []*domainfeed.FeedItem{
		domainfeed.RestoreFeedItem(7, 42, "nickname", "avatar", "title", "description", "media", "cover", 1, 2, 3, publishedAt),
	}}
	recorder := httptest.NewRecorder()
	ctx := newFeedTestContext(http.MethodGet, "/api/feed/time?limit=10", "", recorder)

	NewHandler(applicationfeed.NewService(repository)).Timeline(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("Timeline() status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, fragment := range []string{`"video_id":7`, `"author_nickname":"nickname"`, `"author_avatar_url":"avatar"`, `"description":"description"`} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("Timeline() body = %s, want fragment %s", body, fragment)
		}
	}
}

// TestTimelineRejectsInvalidLimit 验证无效分页参数返回客户端错误。
func TestTimelineRejectsInvalidLimit(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx := newFeedTestContext(http.MethodGet, "/api/feed/time?limit=bad", "", recorder)

	NewHandler(applicationfeed.NewService(&handlerTestRepository{})).Timeline(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("Timeline() status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

// TestRefreshReturnsInternalErrorWhenRepositoryFails 验证仓储错误会转换为服务器错误。
func TestRefreshReturnsInternalErrorWhenRepositoryFails(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx := newFeedTestContext(http.MethodGet, "/api/feed/refresh", "", recorder)
	repository := &handlerTestRepository{err: errors.New("database unavailable")}

	NewHandler(applicationfeed.NewService(repository)).Refresh(ctx)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("Refresh() status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

// TestParseLimit 验证分页参数解析和空值默认行为。
func TestParseLimit(t *testing.T) {
	if limit, err := parseLimit(""); err != nil || limit != 0 {
		t.Fatalf("parseLimit(empty) = (%d, %v), want (0, nil)", limit, err)
	}
	if limit, err := parseLimit(" 12 "); err != nil || limit != 12 {
		t.Fatalf("parseLimit(valid) = (%d, %v), want (12, nil)", limit, err)
	}
	if _, err := parseLimit("0"); !errors.Is(err, domainfeed.ErrInvalidLimit) {
		t.Fatalf("parseLimit(zero) error = %v, want %v", err, domainfeed.ErrInvalidLimit)
	}
}

// newFeedTestContext 创建带 JSON 请求体的 Gin 测试上下文。
func newFeedTestContext(method, target, body string, recorder *httptest.ResponseRecorder) *gin.Context {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = request
	return ctx
}

var _ domainfeed.Repository = (*handlerTestRepository)(nil)

// ListFollowingPage 复用测试时间线结果，满足关注流仓储接口。
func (r *handlerTestRepository) ListFollowingPage(ctx context.Context, _ int64, cursor *domainfeed.TimelineCursor, limit int) ([]*domainfeed.FeedPageItem, error) {
	return r.ListTimelinePage(ctx, cursor, limit)
}

// ListFollowingPullAuthorIDs 返回空作者列表，供非索引测试满足仓储接口。
func (r *handlerTestRepository) ListFollowingPullAuthorIDs(context.Context, int64) ([]int64, error) {
	return nil, nil
}

// ListFollowingAuthorIDs 返回空作者列表，供非索引测试满足仓储接口。
func (r *handlerTestRepository) ListFollowingAuthorIDs(context.Context, int64) ([]int64, error) {
	return nil, nil
}
