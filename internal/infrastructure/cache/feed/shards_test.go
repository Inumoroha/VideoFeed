package feed

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	domaininteraction "github.com/inumoroha/video-feed/internal/domain/interaction"
	"github.com/redis/go-redis/v9"
)

// newShardTestCache 创建隔离的 Redis 模拟服务，测试结束时关闭客户端和服务。
func newShardTestCache(t *testing.T) (*FeedCache, *redis.Client) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewFeedCache(client), client
}

// TestShardedActionConcurrency 验证不同用户并发写同一分片不会丢失增量或覆盖基数。
func TestShardedActionConcurrency(t *testing.T) {
	cache, client := newShardTestCache(t)
	ctx := context.Background()
	const count = 32
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 1; i <= count; i++ {
		wg.Add(1)
		go func(user int64) {
			defer wg.Done()
			_, err := cache.SetActionState(ctx, user*16, 101, "like", true, "first", &domaininteraction.VideoStat{LikeCount: 7})
			errs <- err
		}(int64(i))
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	stat, err := actionStat(ctx, client, interactionStatCounterBaseKey(101), interactionStatCounterShardKeys(101), feedStatKey(101), 101, nil)
	if err != nil || stat.LikeCount != count+7 {
		t.Fatalf("stat=%+v error=%v", stat, err)
	}
	if client.HGet(ctx, interactionStatCounterShardKey(101, 0), "like_count").Val() != "32" {
		t.Fatal("shard lost increments")
	}
}

// TestShardedActionIdempotency 验证旧状态保留、重复请求、取消和传入基数初始化。
func TestShardedActionIdempotency(t *testing.T) {
	cache, client := newShardTestCache(t)
	ctx := context.Background()
	client.Set(ctx, interactionStatCounterKey(102), "obsolete", 0)
	client.HSet(ctx, interactionActionKey(42, 102, "like"), "active", "1", "idempotency_key", "old")
	for _, step := range []struct {
		active bool
		key    string
		delta  int
		count  int
	}{{true, "old", 0, 9}, {false, "cancel", -1, 8}, {false, "cancel", 0, 8}, {true, "new", 1, 9}} {
		result, err := cache.SetActionState(ctx, 42, 102, "like", step.active, step.key, &domaininteraction.VideoStat{VideoID: 102, LikeCount: 9})
		if err != nil {
			t.Fatal(err)
		}
		if result.Delta != step.delta || result.LikeCount != step.count {
			t.Fatalf("result=%+v", result)
		}
	}
	if ttl := client.TTL(ctx, interactionStatCounterBaseKey(102)).Val(); ttl <= 0 || ttl > actionStatTTL {
		t.Fatalf("base ttl=%v", ttl)
	}
	client.Del(ctx, feedStatKey(102))
	stats, err := cache.GetStats(ctx, []int64{102, 999})
	if err != nil || stats[102] == nil || stats[102].LikeCount != 9 || stats[999] != nil {
		t.Fatalf("stats=%+v error=%v", stats, err)
	}
}

// conflictingRedis 在第一次受监视事务执行前修改行为键，模拟并发冲突。
type conflictingRedis struct {
	*redis.Client
	conflict bool
}

// Watch 注入一次外部写入后执行事务，用于验证冲突传递与重试。
func (r *conflictingRedis) Watch(ctx context.Context, fn func(*redis.Tx) error, keys ...string) error {
	return r.Client.Watch(ctx, func(tx *redis.Tx) error {
		if !r.conflict {
			r.conflict = true
			if err := r.Client.HSet(ctx, keys[0], "probe", 1).Err(); err != nil {
				return err
			}
		}
		return fn(tx)
	}, keys...)
}

// TestShardedActionConflictRetry 验证冲突时没有部分写入，重试只增加一次计数。
func TestShardedActionConflictRetry(t *testing.T) {
	_, client := newShardTestCache(t)
	ctx := context.Background()
	cache := NewFeedCache(&conflictingRedis{Client: client})
	if _, err := cache.SetActionState(ctx, 1, 103, "like", true, "key", nil); !errors.Is(err, redis.TxFailedErr) {
		t.Fatalf("error=%v", err)
	}
	if client.Exists(ctx, interactionStatCounterBaseKey(103)).Val() != 0 {
		t.Fatal("failed transaction wrote base")
	}
	result, err := cache.SetActionState(ctx, 1, 103, "like", true, "key", nil)
	if err != nil || result.LikeCount != 1 {
		t.Fatalf("result=%+v error=%v", result, err)
	}
}

// TestShardedStatFallbackAndReadFailure 验证 JSON 回退、负数归零和分片读取失败。
func TestShardedStatFallbackAndReadFailure(t *testing.T) {
	cache, client := newShardTestCache(t)
	ctx := context.Background()
	client.Set(ctx, feedStatKey(104), `{"VideoID":104,"LikeCount":3}`, time.Minute)
	stat, _, err := actionStatFromCache(ctx, client, 104)
	if err != nil || stat.LikeCount != 3 {
		t.Fatalf("stat=%+v error=%v", stat, err)
	}
	client.HSet(ctx, interactionStatCounterBaseKey(104), "like_count", 1)
	client.HSet(ctx, interactionStatCounterShardKey(104, 0), "like_count", -5)
	stat, _, err = actionStatFromCache(ctx, client, 104)
	if err != nil || stat.LikeCount != 0 {
		t.Fatalf("stat=%+v error=%v", stat, err)
	}
	client.Del(ctx, feedStatKey(104))
	client.Set(ctx, interactionStatCounterShardKey(104, 1), "wrong type", 0)
	if _, err := cache.GetStats(ctx, []int64{104}); err == nil {
		t.Fatal("expected shard read failure")
	}
}

// failingStatWriter 模拟聚合成功但 JSON 回填写入失败的缓存。
type failingStatWriter struct{ *actionStatFakeRedis }

// Set 返回写入错误，模拟只读缓存或写入失败。
func (r *failingStatWriter) Set(ctx context.Context, key string, value any, ttl time.Duration) *redis.StatusCmd {
	return redis.NewStatusResult("", errors.New("write unavailable"))
}

// TestStatsIgnoreJSONWriteFailure 验证回填失败不会丢弃已聚合的统计数据。
func TestStatsIgnoreJSONWriteFailure(t *testing.T) {
	client := &failingStatWriter{newActionStatFakeRedis()}
	client.hashes[interactionStatCounterBaseKey(105)] = map[string]string{"favorite_count": "2"}
	stats, err := getStats(context.Background(), client, []int64{105})
	if err != nil || stats[105] == nil || stats[105].FavoriteCount != 2 {
		t.Fatalf("stats=%+v error=%v", stats, err)
	}
}

// TestFavoriteReplayKeepsOriginalState 验证收藏请求的幂等键重放保留原状态且不重复累加。
func TestFavoriteReplayKeepsOriginalState(t *testing.T) {
	cache, client := newShardTestCache(t)
	ctx := context.Background()
	first, err := cache.SetActionState(ctx, 2, 106, "favorite", true, " key ", nil)
	if err != nil || first.FavoriteCount != 1 {
		t.Fatalf("first=%+v error=%v", first, err)
	}
	replay, err := cache.SetActionState(ctx, 2, 106, "favorite", false, "key", nil)
	if err != nil || !replay.Active || replay.Delta != 0 || replay.FavoriteCount != 1 {
		t.Fatalf("replay=%+v error=%v", replay, err)
	}
	if client.HGet(ctx, interactionStatCounterShardKey(106, 2), "favorite_count").Val() != "1" {
		t.Fatal("unexpected favorite increment")
	}
}

// TestStatsIgnoreObsoleteCounter 验证旧哈希不再参与回退，JSON 和初始统计仍可作为来源。
func TestStatsIgnoreObsoleteCounter(t *testing.T) {
	cache, client := newShardTestCache(t)
	ctx := context.Background()
	const videoID int64 = 107
	if err := client.HSet(ctx, interactionStatCounterKey(videoID), "like_count", 99).Err(); err != nil {
		t.Fatal(err)
	}
	stats, err := cache.GetStats(ctx, []int64{videoID})
	if err != nil || stats[videoID] != nil {
		t.Fatalf("obsolete counter used: stats=%+v error=%v", stats, err)
	}
	initial := &domaininteraction.VideoStat{VideoID: videoID, LikeCount: 4}
	stat, err := actionStat(ctx, client, interactionStatCounterBaseKey(videoID), interactionStatCounterShardKeys(videoID), feedStatKey(videoID), videoID, initial)
	if err != nil || stat.LikeCount != 4 {
		t.Fatalf("stat=%+v error=%v", stat, err)
	}
	if err := client.Set(ctx, feedStatKey(videoID), `{"VideoID":107,"LikeCount":6}`, time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	stat, err = actionStat(ctx, client, interactionStatCounterBaseKey(videoID), interactionStatCounterShardKeys(videoID), feedStatKey(videoID), videoID, initial)
	if err != nil || stat.LikeCount != 6 {
		t.Fatalf("stat=%+v error=%v", stat, err)
	}
	empty := actionStatBaseInit(videoID, nil)
	if empty.VideoID != videoID || empty.LikeCount != 0 || empty.CommentCount != 0 || empty.FavoriteCount != 0 {
		t.Fatalf("empty=%+v", empty)
	}
}
