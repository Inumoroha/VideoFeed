package feed

import (
	"context"
	"testing"

	domaininteraction "github.com/inumoroha/video-feed/internal/domain/interaction"
)

// TestCommentStatSurvivesJSONExpiry 验证评论数更新后不会在短期缓存过期时回退，且保留互动分片。
func TestCommentStatSurvivesJSONExpiry(t *testing.T) {
	cache, client := newShardTestCache(t)
	ctx := context.Background()
	const videoID int64 = 2001

	if _, err := cache.SetActionState(ctx, 7, videoID, "like", true, "like-1", &domaininteraction.VideoStat{VideoID: videoID}); err != nil {
		t.Fatal(err)
	}
	if err := cache.SetVideoStat(ctx, &domaininteraction.VideoStat{VideoID: videoID, CommentCount: 1}); err != nil {
		t.Fatal(err)
	}
	for _, stale := range []bool{false, true} {
		if stale {
			client.Del(ctx, feedStatKey(videoID))
		}
		stats, err := cache.GetStats(ctx, []int64{videoID})
		if err != nil {
			t.Fatal(err)
		}
		stat := stats[videoID]
		if stat == nil || stat.CommentCount != 1 || stat.LikeCount != 1 {
			t.Fatalf("stale=%t stat=%+v", stale, stat)
		}
	}

	if err := cache.SetVideoStat(ctx, &domaininteraction.VideoStat{VideoID: videoID, CommentCount: 2}); err != nil {
		t.Fatal(err)
	}
	stats, err := cache.GetStats(ctx, []int64{videoID})
	if err != nil || stats[videoID] == nil || stats[videoID].CommentCount != 2 || stats[videoID].LikeCount != 1 {
		t.Fatalf("updated stats=%+v error=%v", stats, err)
	}
	if ttl := client.TTL(ctx, interactionStatCounterBaseKey(videoID)).Val(); ttl <= 0 || ttl > actionStatTTL {
		t.Fatalf("base ttl=%v", ttl)
	}
}

// TestCommentStatInitializesMissingBase 验证尚无互动分片时用数据库快照补齐基数。
func TestCommentStatInitializesMissingBase(t *testing.T) {
	cache, client := newShardTestCache(t)
	ctx := context.Background()
	const videoID int64 = 2002
	if err := cache.SetVideoStat(ctx, &domaininteraction.VideoStat{
		VideoID: videoID, LikeCount: 2, CommentCount: 3, FavoriteCount: 1,
	}); err != nil {
		t.Fatal(err)
	}
	client.Del(ctx, feedStatKey(videoID))
	stats, err := cache.GetStats(ctx, []int64{videoID})
	if err != nil || stats[videoID] == nil || stats[videoID].LikeCount != 2 || stats[videoID].CommentCount != 3 || stats[videoID].FavoriteCount != 1 {
		t.Fatalf("stats=%+v error=%v", stats, err)
	}
}
