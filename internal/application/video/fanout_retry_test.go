package video_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	applicationvideo "github.com/inumoroha/video-feed/internal/application/video"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"
	feedcache "github.com/inumoroha/video-feed/internal/infrastructure/cache/feed"
	"github.com/redis/go-redis/v9"
)

// failingFanoutRepo 在粉丝分页或预热读取阶段注入一次故障。
type failingFanoutRepo struct {
	memoryFanoutRepo
	stage  string
	failed bool
}

// CountFollowers 按测试阶段返回统计错误或真实测试粉丝数。
func (r *failingFanoutRepo) CountFollowers(ctx context.Context, id int64) (int, error) {
	if r.stage == "count" && !r.failed {
		r.failed = true
		return 0, errors.New("count unavailable")
	}
	return r.memoryFanoutRepo.CountFollowers(ctx, id)
}

// ListFollowerIDs 在第二批注入故障，验证首批成功后的整条消息重试。
func (r *failingFanoutRepo) ListFollowerIDs(ctx context.Context, id, cursor int64, limit int) ([]int64, error) {
	if r.stage == "followers" && cursor > 0 && !r.failed {
		r.failed = true
		return nil, errors.New("batch unavailable")
	}
	return r.memoryFanoutRepo.ListFollowerIDs(ctx, id, cursor, limit)
}

// BatchGetFeedCards 模拟卡片数据库首次读取失败。
func (r *failingFanoutRepo) BatchGetFeedCards(ctx context.Context, ids []int64) (map[int64]*domainfeed.FeedCard, error) {
	if r.stage == "cards" && !r.failed {
		r.failed = true
		return nil, errors.New("cards unavailable")
	}
	return r.memoryFanoutRepo.BatchGetFeedCards(ctx, ids)
}

// BatchGetFeedStats 模拟统计数据库首次读取失败。
func (r *failingFanoutRepo) BatchGetFeedStats(ctx context.Context, ids []int64) (map[int64]*domainfeed.FeedStat, error) {
	if r.stage == "stats" && !r.failed {
		r.failed = true
		return nil, errors.New("stats unavailable")
	}
	return r.memoryFanoutRepo.BatchGetFeedStats(ctx, ids)
}

// TestFanoutPartialFailureRetry 验证各阶段失败会传播，重试可补齐粉丝索引且重复投递不产生重复条目。
func TestFanoutPartialFailureRetry(t *testing.T) {
	for _, stage := range []string{"count", "followers", "cards", "stats"} {
		t.Run(stage, func(t *testing.T) {
			server := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: server.Addr()})
			defer client.Close()
			cache := feedcache.NewFeedCache(client)
			repo := &failingFanoutRepo{memoryFanoutRepo: memoryFanoutRepo{followerCount: 3, followerIDs: []int64{10, 11, 12}}, stage: stage}
			worker := applicationvideo.NewFanoutWorker(repo, nil, cache, applicationvideo.NewFeedPreheater(repo, cache), applicationvideo.WithFanoutBatchSize(2))
			event := &applicationvideo.PublishedEvent{EventID: "same-event", VideoID: 99, AuthorID: 7, PublishedAt: time.Now().UTC()}
			ctx := context.Background()
			if err := worker.HandleVideoPublished(ctx, event); err == nil {
				t.Fatal("expected injected failure")
			}
			for i := 0; i < 2; i++ {
				if err := worker.HandleVideoPublished(ctx, event); err != nil {
					t.Fatal(err)
				}
			}
			for _, id := range []int64{10, 11, 12} {
				items, hit, err := cache.ListFollowingIndexPage(ctx, id, []int64{7}, nil, 5)
				if err != nil || !hit || len(items) != 1 || items[0].VideoID != 99 {
					t.Fatalf("user=%d items=%+v hit=%v err=%v", id, items, hit, err)
				}
			}
			if ttl := client.TTL(ctx, "video:card:v1:99").Val(); ttl <= 14*time.Minute || ttl > 15*time.Minute {
				t.Fatalf("card ttl=%s", ttl)
			}
			if ttl := client.TTL(ctx, "video:stat:v1:99").Val(); ttl <= 0 || ttl > 15*time.Second {
				t.Fatalf("stat ttl=%s", ttl)
			}
		})
	}
}

// TestPublishedEventFields 验证发布事件完整字段、去空白、时间归一化与随机编号。
func TestPublishedEventFields(t *testing.T) {
	if applicationvideo.NewPublishedEvent(nil) != nil || applicationvideo.NewPublishedEvent(&domainvideo.Video{}) != nil {
		t.Fatal("invalid publication accepted")
	}
	at := time.Date(2026, 5, 15, 12, 0, 0, 0, time.FixedZone("test", 8*3600))
	video := &domainvideo.Video{ID: 99, AuthorID: 7, Title: " title ", Description: " description ", MediaURL: " media ", CoverURL: " cover ", PublishedAt: &at}
	e := applicationvideo.NewPublishedEvent(video)
	other := applicationvideo.NewPublishedEvent(video)
	if len(e.EventID) != 24 || e.EventID == other.EventID || e.VideoID != 99 || e.AuthorID != 7 || e.Title != "title" || e.Description != "description" || e.MediaURL != "media" || e.CoverURL != "cover" || !e.PublishedAt.Equal(at) || e.PublishedAt.Location() != time.UTC || e.OccurredAt.IsZero() {
		t.Fatalf("event=%+v", e)
	}
}
