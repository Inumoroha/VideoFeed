package router

import (
	"context"
	"errors"
	"github.com/alicebob/miniredis/v2"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	cachefeed "github.com/inumoroha/video-feed/internal/infrastructure/cache/feed"
	"github.com/redis/go-redis/v9"
	"sync"
	"testing"
	"time"
)

// backfillSource 提供固定作品和可注入错误的仓储读取。
type backfillSource struct{ err error }

// CountFollowers 返回小作者粉丝数或指定错误。
func (s *backfillSource) CountFollowers(context.Context, int64) (int, error) { return 3, s.err }

// ListAuthorRecentVideos 返回固定作品或指定错误。
func (s *backfillSource) ListAuthorRecentVideos(context.Context, int64, int) ([]*domainfeed.FeedPageItem, error) {
	return []*domainfeed.FeedPageItem{{VideoID: 101, AuthorID: 77, PublishedAt: time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)}}, s.err
}

// TestFollowBackfillAdapterConcurrency 验证适配器透传、并发重试去重、容量与有效期、小作者保留、取关过滤和 Redis 故障。
func TestFollowBackfillAdapterConcurrency(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	defer client.Close()
	cache := cachefeed.NewFeedCache(client)
	source := &backfillSource{}
	b := NewFollowFeedBackfiller(source, cache)
	ctx := context.Background()
	count, err := b.CountFollowers(ctx, 77)
	if err != nil || count != 3 {
		t.Fatal(count, err)
	}
	items, err := b.ListAuthorRecentVideos(ctx, 77, 100)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := b.AddInboxItems(ctx, 77, []int64{42}, items[0], 1000); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if n := client.ZCard(ctx, "feed:following:inbox:v1:42").Val(); n != 1 {
		t.Fatal(n)
	}
	if ttl := client.TTL(ctx, "feed:following:inbox:v1:42").Val(); ttl < 29*24*time.Hour {
		t.Fatal(ttl)
	}
	page, hit, err := cache.ListFollowingIndexPage(ctx, 42, []int64{77}, nil, 10)
	if err != nil || !hit || len(page) != 1 {
		t.Fatal(page, hit, err)
	}
	page, _, err = cache.ListFollowingIndexPage(ctx, 42, nil, nil, 10)
	if err != nil || len(page) != 0 {
		t.Fatal(page, err)
	}
	source.err = errors.New("db")
	if _, err = b.CountFollowers(ctx, 77); !errors.Is(err, source.err) {
		t.Fatal(err)
	}
	if _, err = b.ListAuthorRecentVideos(ctx, 77, 100); !errors.Is(err, source.err) {
		t.Fatal(err)
	}
	client.Close()
	if err = b.AddInboxItems(ctx, 77, []int64{42}, items[0], 1000); err == nil {
		t.Fatal("缓存故障未传播")
	}
}
