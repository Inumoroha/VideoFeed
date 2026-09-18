package feed

import (
	"context"
	"sync"
	"testing"
	"time"

	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
)

// TestFollowingIndexConcurrency 验证并发重复投递去重、原子裁剪、有效期、用户隔离和缓存过期。
func TestFollowingIndexConcurrency(t *testing.T) {
	cache, client := newShardTestCache(t)
	ctx := context.Background()
	at := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	for i := 1; i <= 32; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			item := &domainfeed.FeedPageItem{VideoID: id, AuthorID: 7, PublishedAt: at.Add(time.Duration(id) * time.Second)}
			for n := 0; n < 2; n++ {
				if err := cache.AddInboxItems(ctx, 7, []int64{42, -1}, item, 10); err != nil {
					t.Error(err)
				}
			}
		}(int64(i))
	}
	wg.Wait()
	if n := client.ZCard(ctx, followingInboxKey(42)).Val(); n != 10 {
		t.Fatalf("cardinality=%d", n)
	}
	if ttl := client.TTL(ctx, followingInboxKey(42)).Val(); ttl < 29*24*time.Hour {
		t.Fatalf("ttl=%s", ttl)
	}
	items, hit, err := cache.ListFollowingIndexPage(ctx, 42, []int64{7}, nil, 20)
	if err != nil || !hit || len(items) != 10 || items[0].VideoID != 32 || items[9].VideoID != 23 {
		t.Fatalf("items=%+v hit=%v err=%v", items, hit, err)
	}
	items, hit, err = cache.ListFollowingIndexPage(ctx, 43, []int64{7}, nil, 20)
	if err != nil || hit || len(items) != 0 {
		t.Fatalf("cross-user index: %+v %v %v", items, hit, err)
	}
	_ = client.Del(ctx, followingInboxKey(42)).Err()
	_, hit, err = cache.ListFollowingIndexPage(ctx, 42, []int64{7}, nil, 20)
	if err != nil || hit {
		t.Fatalf("missing index: %v %v", hit, err)
	}
}

// TestFollowingIndexMergeCursorAndUnfollow 验证普通作者保留、取关过滤、跨来源去重及同秒纳秒与大编号游标排序。
func TestFollowingIndexMergeCursorAndUnfollow(t *testing.T) {
	cache, _ := newShardTestCache(t)
	ctx := context.Background()
	at := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	inputs := []*domainfeed.FeedPageItem{
		{VideoID: 2000001, AuthorID: 7, PublishedAt: at},
		{VideoID: 2, AuthorID: 7, PublishedAt: at},
		{VideoID: 1, AuthorID: 8, PublishedAt: at.Add(1)},
		{VideoID: 3, AuthorID: 9, PublishedAt: at.Add(time.Hour)},
	}
	for _, item := range inputs {
		if err := cache.AddInboxItems(ctx, item.AuthorID, []int64{42}, item, 100); err != nil {
			t.Fatal(err)
		}
		if err := cache.AddAuthorOutboxItem(ctx, item.AuthorID, item, 100); err != nil {
			t.Fatal(err)
		}
	}
	items, hit, err := cache.ListFollowingIndexPage(ctx, 42, []int64{7, 8}, nil, 2)
	if err != nil || !hit || len(items) != 2 || items[0].VideoID != 1 || items[1].VideoID != 2000001 {
		t.Fatalf("items=%+v hit=%v err=%v", items, hit, err)
	}
	items, _, err = cache.ListFollowingIndexPage(ctx, 42, []int64{7, 8}, &domainfeed.TimelineCursor{PublishedAt: at, VideoID: 2000001}, 2)
	if err != nil || len(items) != 1 || items[0].VideoID != 2 {
		t.Fatalf("cursor: %+v %v", items, err)
	}
	items, _, err = cache.ListFollowingIndexPage(ctx, 42, []int64{8}, nil, 10)
	if err != nil || len(items) != 1 || items[0].AuthorID != 8 {
		t.Fatalf("unfollow: %+v %v", items, err)
	}
}

// TestFollowingIndexInvalidAndFailure 验证无效写入无副作用、默认容量及 Redis 断开时错误可传播。
func TestFollowingIndexInvalidAndFailure(t *testing.T) {
	cache, client := newShardTestCache(t)
	ctx := context.Background()
	if err := cache.AddInboxItems(ctx, 0, []int64{42}, nil, 0); err != nil {
		t.Fatal(err)
	}
	if err := cache.AddAuthorOutboxItem(ctx, 0, nil, 0); err != nil {
		t.Fatal(err)
	}
	item := &domainfeed.FeedPageItem{VideoID: 1, AuthorID: 7, PublishedAt: time.Now()}
	if err := cache.AddAuthorOutboxItem(ctx, 7, item, 0); err != nil {
		t.Fatal(err)
	}
	if client.ZCard(ctx, followingAuthorOutboxKey(7)).Val() != 1 {
		t.Fatal("default outbox missing")
	}
	for _, member := range []string{"bad", "0:7:bad", "1:7:bad"} {
		if _, ok := feedPageItemFromFollowingMember(member); ok {
			t.Fatalf("accepted %q", member)
		}
	}
	_ = client.Close()
	if err := cache.AddInboxItems(ctx, 7, []int64{42}, item, 0); err == nil {
		t.Fatal("write failure swallowed")
	}
	if _, _, err := cache.ListFollowingIndexPage(ctx, 42, []int64{7}, nil, 2); err == nil {
		t.Fatal("read failure swallowed")
	}
}
