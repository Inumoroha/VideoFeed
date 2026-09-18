package feed

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
)

// followingTestRepository 模拟按用户隔离的关注页和各阶段读取错误。
type followingTestRepository struct {
	serviceTestRepository
	pageErr, cardErr, statErr error
}

// ListFollowingPage 按用户返回唯一视频，并传播上下文取消或预设故障。
func (r *followingTestRepository) ListFollowingPage(ctx context.Context, viewerID int64, _ *domainfeed.TimelineCursor, _ int) ([]*domainfeed.FeedPageItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []*domainfeed.FeedPageItem{{VideoID: viewerID, PublishedAt: time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)}}, r.pageErr
}

// BatchGetFeedCards 返回测试卡片或预设的回源错误。
func (r *followingTestRepository) BatchGetFeedCards(ctx context.Context, ids []int64) (map[int64]*domainfeed.FeedCard, error) {
	items, _ := r.serviceTestRepository.BatchGetFeedCards(ctx, ids)
	return items, r.cardErr
}

// BatchGetFeedStats 返回空统计或预设的回源错误。
func (r *followingTestRepository) BatchGetFeedStats(context.Context, []int64) (map[int64]*domainfeed.FeedStat, error) {
	return map[int64]*domainfeed.FeedStat{}, r.statErr
}

// failingFollowingCache 模拟卡片和计数缓存不可用，并拒绝共享关注页。
type failingFollowingCache struct{ missingFeedCache }

// GetPage 禁止关注流使用不含用户身份的公共页缓存。
func (failingFollowingCache) GetPage(context.Context, string) (*FeedPage, bool, error) {
	panic("following page cache must not be used")
}

// SetPage 禁止关注流写入公共页缓存。
func (failingFollowingCache) SetPage(context.Context, string, *FeedPage, time.Duration) error {
	panic("following page cache must not be used")
}

// GetCards 模拟卡片缓存读取失败。
func (failingFollowingCache) GetCards(context.Context, []int64) (map[int64]*domainfeed.FeedCard, error) {
	return nil, errors.New("cache unavailable")
}

// GetStats 模拟计数缓存读取失败。
func (failingFollowingCache) GetStats(context.Context, []int64) (map[int64]*domainfeed.FeedStat, error) {
	return nil, errors.New("cache unavailable")
}

// SetCards 模拟卡片缓存回填失败。
func (failingFollowingCache) SetCards(context.Context, map[int64]*domainfeed.FeedCard, time.Duration) error {
	return errors.New("cache unavailable")
}

// SetStats 模拟计数缓存回填失败。
func (failingFollowingCache) SetStats(context.Context, map[int64]*domainfeed.FeedStat, time.Duration) error {
	return errors.New("cache unavailable")
}

// TestFollowingValidation 验证关注流认证、游标、分页参数和空仓储检查。
func TestFollowingValidation(t *testing.T) {
	s := NewFollowingStrategy(&followingTestRepository{})
	for _, tc := range []struct {
		req  FeedRequest
		want error
	}{
		{FeedRequest{}, domainfeed.ErrViewerRequired},
		{FeedRequest{ViewerID: -1}, domainfeed.ErrViewerRequired},
		{FeedRequest{ViewerID: 42, Cursor: "bad"}, domainfeed.ErrInvalidCursor},
		{FeedRequest{ViewerID: 42, Limit: -1}, domainfeed.ErrInvalidLimit},
	} {
		if _, err := s.List(context.Background(), tc.req); !errors.Is(err, tc.want) {
			t.Fatalf("request=%+v error=%v", tc.req, err)
		}
	}
	if _, err := NewFollowingStrategy(nil).List(context.Background(), FeedRequest{ViewerID: 42}); !errors.Is(err, ErrFeedServiceNotInitialized) {
		t.Fatal(err)
	}
}

// TestFollowingFailureAndRetry 验证各读取阶段失败映射、取消传播及下一次请求可正常重试。
func TestFollowingFailureAndRetry(t *testing.T) {
	failure := errors.New("database unavailable")
	for _, repo := range []*followingTestRepository{{pageErr: failure}, {cardErr: failure}, {statErr: failure}} {
		s := NewService(repo, WithFeedCache(failingFollowingCache{}))
		req := FeedRequest{Scene: domainfeed.SceneFollowing, ViewerID: 42}
		if _, err := s.GetFeed(context.Background(), req); !errors.Is(err, ErrLoadFeedFailed) {
			t.Fatal(err)
		}
		repo.pageErr, repo.cardErr, repo.statErr = nil, nil, nil
		page, err := s.GetFeed(context.Background(), req)
		if err != nil || len(page.Items) != 1 || page.HasMore || page.NextCursor == "" {
			t.Fatalf("page=%+v err=%v", page, err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := s.GetFeed(ctx, req); !errors.Is(err, ErrLoadFeedFailed) {
			t.Fatal(err)
		}
	}
}

// TestFollowingConcurrentViewersAndCacheFallback 验证缓存故障下并发用户分别回源，关注页不会串用户。
func TestFollowingConcurrentViewersAndCacheFallback(t *testing.T) {
	s := NewService(&followingTestRepository{}, WithFeedCache(failingFollowingCache{}))
	if s.strategies[domainfeed.SceneFollowing].(*FollowingStrategy).cache == nil {
		t.Fatal("following cache not injected")
	}
	var wg sync.WaitGroup
	for id := int64(1); id <= 32; id++ {
		wg.Add(1)
		go func(viewerID int64) {
			defer wg.Done()
			page, err := s.GetFeed(context.Background(), FeedRequest{Scene: domainfeed.SceneFollowing, ViewerID: viewerID})
			if err != nil || len(page.Items) != 1 || page.Items[0].VideoID != viewerID {
				t.Errorf("viewer=%d page=%+v err=%v", viewerID, page, err)
			}
		}(id)
	}
	wg.Wait()
}

// TestFollowingLookaheadAndEmptyPage 验证最大页大小、多取一条、游标内容和空页行为。
func TestFollowingLookaheadAndEmptyPage(t *testing.T) {
	r := &serviceTestRepository{items: makeFeedItems(domainfeed.MaxLimit + 1)}
	s := NewFollowingStrategy(r)
	page, err := s.List(context.Background(), FeedRequest{ViewerID: 42, Limit: 999})
	if err != nil || r.listLimit != domainfeed.MaxLimit+1 || len(page.Items) != domainfeed.MaxLimit || !page.HasMore {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	cursor, err := parseTimelineCursor(page.NextCursor)
	last := r.items[domainfeed.MaxLimit-1]
	if err != nil || cursor.VideoID != last.VideoID || !cursor.PublishedAt.Equal(last.PublishedAt) {
		t.Fatalf("cursor=%+v err=%v", cursor, err)
	}
	r.items = nil
	page, err = s.List(context.Background(), FeedRequest{ViewerID: 42, Cursor: page.NextCursor})
	if err != nil || r.listCursor.VideoID != cursor.VideoID || r.listLimit != defaultFeedLimit+1 || len(page.Items) != 0 || page.NextCursor != "" || page.HasMore {
		t.Fatalf("page=%+v err=%v", page, err)
	}
}
