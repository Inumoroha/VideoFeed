package feed

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"testing"
	"time"

	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
)

// serviceTestRepository 提供信息流应用服务测试所需的内存仓储实现。
type serviceTestRepository struct {
	items      []*domainfeed.FeedItem
	listLimit  int
	listCursor *domainfeed.TimelineCursor
	err        error
}

func (r *serviceTestRepository) ListTimelinePage(_ context.Context, c *domainfeed.TimelineCursor, limit int) ([]*domainfeed.FeedPageItem, error) {
	r.listLimit = limit
	r.listCursor = c
	if r.err != nil {
		return nil, r.err
	}
	out := make([]*domainfeed.FeedPageItem, 0, len(r.items))
	for _, i := range r.items {
		out = append(out, &domainfeed.FeedPageItem{VideoID: i.VideoID, PublishedAt: i.PublishedAt, HotScore: i.HotScore})
	}
	return out, r.err
}
func (r *serviceTestRepository) ListHotPage(_ context.Context, _ *domainfeed.HotCursor, limit int) ([]*domainfeed.FeedPageItem, error) {
	return nil, r.err
}
func (r *serviceTestRepository) BatchGetFeedCards(_ context.Context, ids []int64) (map[int64]*domainfeed.FeedCard, error) {
	m := map[int64]*domainfeed.FeedCard{}
	for _, id := range ids {
		m[id] = &domainfeed.FeedCard{VideoID: id}
	}
	return m, nil
}
func (r *serviceTestRepository) BatchGetFeedStats(_ context.Context, ids []int64) (map[int64]*domainfeed.FeedStat, error) {
	return map[int64]*domainfeed.FeedStat{}, nil
}

// ListTimelineFeed 返回测试信息流并记录服务请求的游标和查询数量。
func (r *serviceTestRepository) ListTimelineFeed(_ context.Context, cursor *domainfeed.TimelineCursor, limit int) ([]*domainfeed.FeedItem, error) {
	r.listLimit = limit
	if cursor != nil {
		cloned := *cursor
		r.listCursor = &cloned
	}
	return r.items, r.err
}

// ListHotFeed 返回测试热榜信息流。
func (r *serviceTestRepository) ListHotFeed(_ context.Context, _ *domainfeed.HotCursor, _ int) ([]*domainfeed.FeedItem, error) {
	return r.items, r.err
}

// TestServiceRejectsUninitializedService 验证未初始化服务不会触发空指针异常。
func TestServiceRejectsUninitializedService(t *testing.T) {
	var service *Service
	if _, err := service.GetTimelineFeed(context.TODO(), "", 20); !errors.Is(err, ErrFeedServiceNotInitialized) {
		t.Fatalf("GetTimelineFeed() error = %v, want %v", err, ErrFeedServiceNotInitialized)
	}

	if _, err := NewService(nil).GetTimelineFeed(context.TODO(), "", 20); !errors.Is(err, ErrFeedServiceNotInitialized) {
		t.Fatalf("GetTimelineFeed() error = %v, want %v", err, ErrFeedServiceNotInitialized)
	}
}

// TestGetTimelineFeedUsesLookaheadAndBuildsCursor 验证服务会多查一条数据并生成下一页游标。
func TestGetTimelineFeedUsesLookaheadAndBuildsCursor(t *testing.T) {
	repository := &serviceTestRepository{items: makeFeedItems(domainfeed.MaxLimit + 1)}
	result, err := NewService(repository).GetTimelineFeed(context.TODO(), "", domainfeed.MaxLimit)
	if err != nil {
		t.Fatalf("GetTimelineFeed() error = %v", err)
	}
	if repository.listLimit != domainfeed.MaxLimit+1 {
		t.Fatalf("ListTimelineFeed() limit = %d, want %d", repository.listLimit, domainfeed.MaxLimit+1)
	}
	if len(result.Items) != domainfeed.MaxLimit || !result.HasMore || result.NextCursor == "" {
		t.Fatalf("GetTimelineFeed() result = %+v, want capped items with next cursor", result)
	}
}

// TestGetTimelineFeedOmitsCursorWhenExhausted 验证没有更多数据时不会返回下一页游标。
func TestGetTimelineFeedOmitsCursorWhenExhausted(t *testing.T) {
	repository := &serviceTestRepository{items: makeFeedItems(1)}
	result, err := NewService(repository).GetTimelineFeed(context.TODO(), "", 20)
	if err != nil {
		t.Fatalf("GetTimelineFeed() error = %v", err)
	}
	if result.HasMore || result.NextCursor != "" {
		t.Fatalf("GetTimelineFeed() result = %+v, want no next cursor", result)
	}
}

// TestGetTimelineFeedRejectsNegativeLimit 验证负数分页数量会返回领域校验错误。
func TestGetTimelineFeedRejectsNegativeLimit(t *testing.T) {
	repository := &serviceTestRepository{}
	_, err := NewService(repository).GetTimelineFeed(context.TODO(), "", -1)
	if !errors.Is(err, domainfeed.ErrInvalidLimit) {
		t.Fatalf("GetTimelineFeed() error = %v, want %v", err, domainfeed.ErrInvalidLimit)
	}
	if repository.listLimit != 0 {
		t.Fatalf("ListTimelineFeed() limit = %d, want no repository call", repository.listLimit)
	}
}

// TestGetTimelineFeedNormalizesLimit 验证零值和超大分页数量会被规范化。
func TestGetTimelineFeedNormalizesLimit(t *testing.T) {
	for _, test := range []struct {
		name  string
		input int
		want  int
	}{
		{name: "default", input: 0, want: defaultFeedLimit + 1},
		{name: "maximum", input: domainfeed.MaxLimit + 50, want: domainfeed.MaxLimit + 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &serviceTestRepository{}
			if _, err := NewService(repository).GetTimelineFeed(context.TODO(), "", test.input); err != nil {
				t.Fatalf("GetTimelineFeed() error = %v", err)
			}
			if repository.listLimit != test.want {
				t.Fatalf("ListTimelineFeed() limit = %d, want %d", repository.listLimit, test.want)
			}
		})
	}
}

// TestGetTimelineFeedUsesCursor 验证服务会解码游标并传递给仓储。
func TestGetTimelineFeedUsesCursor(t *testing.T) {
	publishedAt := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	cursor := &domainfeed.TimelineCursor{PublishedAt: publishedAt, VideoID: 7}
	repository := &serviceTestRepository{}

	if _, err := NewService(repository).GetTimelineFeed(context.TODO(), encodeTimelineCursor(cursor), 2); err != nil {
		t.Fatalf("GetTimelineFeed() error = %v", err)
	}
	if repository.listCursor == nil || repository.listCursor.VideoID != cursor.VideoID || !repository.listCursor.PublishedAt.Equal(cursor.PublishedAt) {
		t.Fatalf("ListTimelineFeed() cursor = %+v, want %+v", repository.listCursor, cursor)
	}
}

// TestGetTimelineFeedRejectsInvalidCursor 验证无法解析的游标会被拒绝。
func TestGetTimelineFeedRejectsInvalidCursor(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"published_at":"","video_id":7}`))
	_, err := NewService(&serviceTestRepository{}).GetTimelineFeed(context.TODO(), payload, 20)
	if !errors.Is(err, domainfeed.ErrInvalidCursor) {
		t.Fatalf("GetTimelineFeed() error = %v, want %v", err, domainfeed.ErrInvalidCursor)
	}
}

// TestGetTimelineFeedMapsRepositoryError 验证仓储错误会转换为应用层错误。
func TestGetTimelineFeedMapsRepositoryError(t *testing.T) {
	_, err := NewService(&serviceTestRepository{err: errors.New("database unavailable")}).GetTimelineFeed(context.TODO(), "", 20)
	if !errors.Is(err, ErrLoadFeedFailed) {
		t.Fatalf("GetTimelineFeed() error = %v, want %v", err, ErrLoadFeedFailed)
	}
}

// TestRefreshTimelineFeedStartsFromBeginning 验证刷新查询不会携带旧游标。
func TestRefreshTimelineFeedStartsFromBeginning(t *testing.T) {
	repository := &serviceTestRepository{}
	if _, err := NewService(repository).RefreshTimelineFeed(context.TODO(), 10); err != nil {
		t.Fatalf("RefreshTimelineFeed() error = %v", err)
	}
	if repository.listCursor != nil {
		t.Fatalf("ListTimelineFeed() cursor = %+v, want nil", repository.listCursor)
	}
}

// TestTimelineCursorRoundTrip 验证时间线游标编码后可以无损解析。
func TestTimelineCursorRoundTrip(t *testing.T) {
	original := &domainfeed.TimelineCursor{PublishedAt: time.Date(2026, time.August, 30, 12, 0, 0, 123000000, time.UTC), VideoID: 7}
	decoded, err := parseTimelineCursor(encodeTimelineCursor(original))
	if err != nil {
		t.Fatalf("parseTimelineCursor() error = %v", err)
	}
	if decoded.VideoID != original.VideoID || !decoded.PublishedAt.Equal(original.PublishedAt) {
		t.Fatalf("decoded cursor = %+v, want %+v", decoded, original)
	}
}

// makeFeedItems 创建指定数量的按时间递减信息流测试数据。
func makeFeedItems(count int) []*domainfeed.FeedItem {
	items := make([]*domainfeed.FeedItem, 0, count)
	base := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	for i := 0; i < count; i++ {
		items = append(items, &domainfeed.FeedItem{
			VideoID:     int64(count - i),
			Title:       fmt.Sprintf("video-%d", count-i),
			PublishedAt: base.Add(-time.Duration(i) * time.Minute),
		})
	}
	return items
}

var _ domainfeed.Repository = (*serviceTestRepository)(nil)
