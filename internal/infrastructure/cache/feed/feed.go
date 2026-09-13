package feed

import (
	"context"
	"encoding/json"
	"fmt"
	applicationfeed "github.com/inumoroha/video-feed/internal/application/feed"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	"github.com/redis/go-redis/v9"
	"strconv"
	"strings"
	"time"
)

const hotWindowMinutes = 60
const hotMinuteBucketTTL = 2 * time.Hour
const hotWindowCacheTTL = 2 * time.Minute

// FeedCache 使用 Redis 保存 Feed 查询结果。
type FeedCache struct{ client redis.Cmdable }

// NewFeedCache 创建 Feed 结果缓存。
func NewFeedCache(client redis.Cmdable) *FeedCache { return &FeedCache{client: client} }

// GetPage 读取轻量 Feed 页。
func (c *FeedCache) GetPage(ctx context.Context, key string) (*applicationfeed.FeedPage, bool, error) {
	b, e := c.client.Get(ctx, key).Bytes()
	if e == redis.Nil {
		return nil, false, nil
	}
	if e != nil {
		return nil, false, e
	}
	var p applicationfeed.FeedPage
	if e = json.Unmarshal(b, &p); e != nil {
		return nil, false, e
	}
	return &p, true, nil
}

// SetPage 写入轻量 Feed 页。
func (c *FeedCache) SetPage(ctx context.Context, key string, p *applicationfeed.FeedPage, ttl time.Duration) error {
	b, e := json.Marshal(p)
	if e != nil {
		return e
	}
	return c.client.Set(ctx, key, b, ttl).Err()
}

// GetCards 批量读取视频卡片缓存。
func (c *FeedCache) GetCards(ctx context.Context, ids []int64) (map[int64]*domainfeed.FeedCard, error) {
	return c.getCards(ctx, ids, feedCardKey)
}

// SetCards 批量写入视频卡片缓存。
func (c *FeedCache) SetCards(ctx context.Context, m map[int64]*domainfeed.FeedCard, ttl time.Duration) error {
	for id, v := range m {
		if v == nil || v.VideoID <= 0 {
			continue
		}
		b, e := json.Marshal(v)
		if e != nil {
			return e
		}
		if e = c.client.Set(ctx, feedCardKey(id), b, ttl).Err(); e != nil {
			return e
		}
	}
	return nil
}

// GetStats 批量读取视频计数缓存。
func (c *FeedCache) GetStats(ctx context.Context, ids []int64) (map[int64]*domainfeed.FeedStat, error) {
	m := map[int64]*domainfeed.FeedStat{}
	if len(ids) == 0 {
		return m, nil
	}
	vs, e := c.client.MGet(ctx, cacheKeys(ids, feedStatKey)...).Result()
	if e != nil {
		return nil, e
	}
	for i, v := range vs {
		b, ok := cacheValueBytes(v)
		if !ok {
			continue
		}
		var s domainfeed.FeedStat
		if json.Unmarshal(b, &s) != nil {
			continue
		}
		if s.VideoID <= 0 {
			s.VideoID = ids[i]
		}
		m[s.VideoID] = &s
	}
	return m, nil
}

// SetStats 批量写入视频计数缓存。
func (c *FeedCache) SetStats(ctx context.Context, m map[int64]*domainfeed.FeedStat, ttl time.Duration) error {
	for id, v := range m {
		if v == nil || v.VideoID <= 0 {
			continue
		}
		b, e := json.Marshal(v)
		if e != nil {
			return e
		}
		if e = c.client.Set(ctx, feedStatKey(id), b, ttl).Err(); e != nil {
			return e
		}
	}
	return nil
}

// AddHotScore 将互动热度写入分钟粒度的 Redis ZSET。
func (c *FeedCache) AddHotScore(ctx context.Context, videoID int64, delta int, at time.Time) error {
	if videoID <= 0 || delta == 0 {
		return nil
	}
	key := hotMinuteKey(at)
	if err := c.client.ZIncrBy(ctx, key, float64(delta), fmt.Sprintf("%020d", videoID)).Err(); err != nil {
		return err
	}
	return c.client.Expire(ctx, key, hotMinuteBucketTTL).Err()
}

// ListHotWindowPage 合并最近一小时热度桶并读取分页结果。
func (c *FeedCache) ListHotWindowPage(ctx context.Context, end time.Time, offset, limit int) ([]*domainfeed.FeedPageItem, error) {
	if limit <= 0 {
		return []*domainfeed.FeedPageItem{}, nil
	}
	end = end.UTC().Truncate(time.Minute)
	key := fmt.Sprintf("feed:hot:window:v1:%d", end.Unix())
	if _, e := c.client.ZUnionStore(ctx, key, &redis.ZStore{Keys: hotWindowMinuteKeys(end), Aggregate: "SUM"}).Result(); e != nil {
		return nil, e
	}
	_ = c.client.Expire(ctx, key, hotWindowCacheTTL).Err()
	vs, e := c.client.ZRevRangeWithScores(ctx, key, int64(offset), int64(offset+limit-1)).Result()
	if e != nil {
		return nil, e
	}
	out := make([]*domainfeed.FeedPageItem, 0, len(vs))
	for _, v := range vs {
		m, ok := v.Member.(string)
		if !ok {
			continue
		}
		id, _ := strconv.ParseInt(strings.TrimLeft(m, "0"), 10, 64)
		if id > 0 {
			out = append(out, &domainfeed.FeedPageItem{VideoID: id, HotScore: int(v.Score)})
		}
	}
	return out, nil
}
func hotMinuteKey(t time.Time) string {
	return fmt.Sprintf("feed:hot:minute:v1:%s", t.UTC().Truncate(time.Minute).Format("200601021504"))
}
func hotWindowMinuteKeys(e time.Time) []string {
	r := make([]string, 0, hotWindowMinutes)
	for i := hotWindowMinutes - 1; i >= 0; i-- {
		r = append(r, hotMinuteKey(e.Add(-time.Duration(i)*time.Minute)))
	}
	return r
}
func (c *FeedCache) getCards(ctx context.Context, ids []int64, key func(int64) string) (map[int64]*domainfeed.FeedCard, error) {
	m := map[int64]*domainfeed.FeedCard{}
	if len(ids) == 0 {
		return m, nil
	}
	vs, e := c.client.MGet(ctx, cacheKeys(ids, key)...).Result()
	if e != nil {
		return nil, e
	}
	for i, v := range vs {
		b, ok := cacheValueBytes(v)
		if !ok {
			continue
		}
		var x domainfeed.FeedCard
		if json.Unmarshal(b, &x) != nil {
			continue
		}
		if x.VideoID <= 0 {
			x.VideoID = ids[i]
		}
		m[x.VideoID] = &x
	}
	return m, nil
}
func cacheKeys(ids []int64, b func(int64) string) []string {
	r := make([]string, 0, len(ids))
	for _, id := range ids {
		r = append(r, b(id))
	}
	return r
}
func feedCardKey(id int64) string { return fmt.Sprintf("video:card:v1:%d", id) }
func feedStatKey(id int64) string { return fmt.Sprintf("video:stat:v1:%d", id) }
func cacheValueBytes(v any) ([]byte, bool) {
	switch x := v.(type) {
	case string:
		return []byte(x), true
	case []byte:
		return x, true
	default:
		return nil, false
	}
}
