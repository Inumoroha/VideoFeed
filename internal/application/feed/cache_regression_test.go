package feed

import (
	"context"
	"encoding/base64"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
)

// blockingFeedRepository 在测试主动放行或上下文取消之前阻塞时间线查询。
type blockingFeedRepository struct {
	serviceTestRepository
	started chan context.Context
	release chan struct{}
	calls   atomic.Int32
}

func (r *blockingFeedRepository) ListTimelinePage(ctx context.Context, c *domainfeed.TimelineCursor, l int) ([]*domainfeed.FeedPageItem, error) {
	r.calls.Add(1)
	r.started <- ctx
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-r.release:
		return []*domainfeed.FeedPageItem{{VideoID: 1, PublishedAt: time.Now()}}, nil
	}
}

// ListTimelineFeed 记录共享查询上下文并等待放行，用于验证取消隔离。
func (r *blockingFeedRepository) ListTimelineFeed(ctx context.Context, _ *domainfeed.TimelineCursor, _ int) ([]*domainfeed.FeedItem, error) {
	r.calls.Add(1)
	r.started <- ctx
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-r.release:
		return makeFeedItems(1), nil
	}
}

// missingFeedCache 模拟始终未命中的缓存。
type missingFeedCache struct{}

func (missingFeedCache) GetPage(context.Context, string) (*FeedPage, bool, error) {
	return nil, false, nil
}
func (missingFeedCache) SetPage(context.Context, string, *FeedPage, time.Duration) error { return nil }
func (missingFeedCache) GetCards(context.Context, []int64) (map[int64]*domainfeed.FeedCard, error) {
	return map[int64]*domainfeed.FeedCard{}, nil
}
func (missingFeedCache) SetCards(context.Context, map[int64]*domainfeed.FeedCard, time.Duration) error {
	return nil
}
func (missingFeedCache) GetStats(context.Context, []int64) (map[int64]*domainfeed.FeedStat, error) {
	return map[int64]*domainfeed.FeedStat{}, nil
}
func (missingFeedCache) SetStats(context.Context, map[int64]*domainfeed.FeedStat, time.Duration) error {
	return nil
}

// Get 返回缓存未命中。
func (missingFeedCache) Get(context.Context, string) (*FeedResult, bool, error) {
	return nil, false, nil
}

// Set 接受缓存写入而不保存数据。
func (missingFeedCache) Set(context.Context, string, *FeedResult, time.Duration) error {
	return nil
}

// observedWaitContext 在请求进入取消等待时通知测试，避免通过睡眠猜测并发顺序。
type observedWaitContext struct {
	context.Context
	waiting chan struct{}
}

// Done 通知测试当前请求已注册共享查询并准备等待结果。
func (c observedWaitContext) Done() <-chan struct{} {
	select {
	case c.waiting <- struct{}{}:
	default:
	}
	return c.Context.Done()
}

// TestTimelineCacheCancellationIsolation 验证首请求和等待者均可取消且不终止共享查询。
func TestTimelineCacheCancellationIsolation(t *testing.T) {
	repo := &blockingFeedRepository{started: make(chan context.Context, 3), release: make(chan struct{})}
	defer close(repo.release)
	service := NewService(repo, WithTimelineCache(missingFeedCache{}))
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	firstResult := make(chan error, 1)
	go func() {
		_, err := service.GetTimelineFeed(firstCtx, "", 10)
		firstResult <- err
	}()
	var sharedCtx context.Context
	select {
	case sharedCtx = <-repo.started:
	case <-time.After(time.Second):
		t.Fatal("repository did not start")
	}
	if _, ok := sharedCtx.Deadline(); !ok {
		t.Fatal("shared query has no deadline")
	}
	cancelFirst()
	select {
	case err := <-firstResult:
		if !(errors.Is(err, context.Canceled) || errors.Is(err, ErrLoadFeedFailed)) {
			t.Fatalf("first request: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("first request did not stop waiting")
	}
	if err := sharedCtx.Err(); err != nil {
		t.Fatalf("shared query canceled: %v", err)
	}

	waiterCtx, cancelWaiter := context.WithCancel(context.Background())
	defer cancelWaiter()
	waiting := make(chan struct{}, 1)
	waiterResult := make(chan error, 1)
	go func() {
		_, err := service.GetTimelineFeed(observedWaitContext{waiterCtx, waiting}, "", 10)
		waiterResult <- err
	}()
	select {
	case <-waiting:
	case <-time.After(time.Second):
		t.Fatal("waiter did not join")
	}
	cancelWaiter()
	select {
	case err := <-waiterResult:
		if !(errors.Is(err, context.Canceled) || errors.Is(err, ErrLoadFeedFailed)) {
			t.Fatalf("waiter: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiter did not cancel")
	}
	if err := sharedCtx.Err(); err != nil {
		t.Fatalf("shared query canceled: %v", err)
	}

	healthyWaiting := make(chan struct{}, 1)
	healthyResult := make(chan error, 1)
	go func() {
		result, err := service.GetTimelineFeed(observedWaitContext{context.Background(), healthyWaiting}, "", 10)
		if err == nil && (result == nil || len(result.Items) != 1) {
			err = errors.New("unexpected feed result")
		}
		healthyResult <- err
	}()
	select {
	case <-healthyWaiting:
	case <-time.After(time.Second):
		t.Fatal("healthy request did not join")
	}
	repo.release <- struct{}{}
	select {
	case err := <-healthyResult:
		if err != nil {
			t.Fatalf("healthy request failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("healthy request did not complete")
	}
	if calls := repo.calls.Load(); calls != 1 {
		t.Fatalf("repository calls = %d, want 1", calls)
	}
}

// TestFeedRejectsZeroTimeCursor 验证两个场景均将零时间游标作为参数错误拒绝。
func TestFeedRejectsZeroTimeCursor(t *testing.T) {
	cursor := base64.RawURLEncoding.EncodeToString([]byte(`{"published_at":"0001-01-01T00:00:00Z","video_id":1,"hot_score":0}`))
	for _, scene := range []domainfeed.Scene{domainfeed.SceneTimeline, domainfeed.SceneHot} {
		t.Run(string(scene), func(t *testing.T) {
			service := NewService(&serviceTestRepository{err: errors.New("repository must not be used")})
			_, err := service.GetFeed(context.Background(), FeedRequest{Scene: scene, Cursor: cursor})
			if !errors.Is(err, domainfeed.ErrInvalidCursor) {
				t.Fatalf("error = %v, want invalid cursor", err)
			}
		})
	}
}
