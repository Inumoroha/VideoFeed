package feed

import (
	"context"
	"encoding/json"
	"fmt"
	applicationfeed "github.com/inumoroha/video-feed/internal/application/feed"
	applicationinteraction "github.com/inumoroha/video-feed/internal/application/interaction"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	domaininteraction "github.com/inumoroha/video-feed/internal/domain/interaction"
	"github.com/redis/go-redis/v9"
	"strconv"
	"strings"
	"time"
)

// SetActionState 仅监视用户行为键并事务更新分片，提交后聚合计数；冲突和读取失败返回错误。
func (c *FeedCache) SetActionState(ctx context.Context, userID, videoID int64, actionType string, active bool, idempotencyKey string, initial *domaininteraction.VideoStat) (*applicationinteraction.ActionStateResult, error) {
	// Redis Cmdable 在当前实现中通过事务管道完成状态与计数的一致写入。
	var result *applicationinteraction.ActionStateResult
	actionType, err := domaininteraction.NormalizeActionType(actionType)
	if err != nil {
		return nil, err
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	err = c.client.Watch(ctx, func(tx *redis.Tx) error {
		var e error
		result, e = c.setActionStateWithClient(ctx, tx, userID, videoID, actionType, active, idempotencyKey, initial)
		return e
	}, interactionActionKey(userID, videoID, actionType))
	if err != nil {
		return nil, err
	}
	stat, err := actionStat(ctx, c.client, interactionStatCounterBaseKey(videoID), interactionStatCounterShardKeys(videoID), feedStatKey(videoID), videoID, initial)
	if err != nil {
		return nil, err
	}
	result.LikeCount = stat.LikeCount
	result.FavoriteCount = stat.FavoriteCount
	_ = setActionStatJSON(ctx, c.client, feedStatKey(videoID), stat)
	return result, nil
}

// setActionStateWithClient 在受监视事务中保存行为并原子累加分片，重复幂等键不增加计数。
func (c *FeedCache) setActionStateWithClient(ctx context.Context, client redis.Cmdable, userID, videoID int64, actionType string, active bool, idempotencyKey string, initial *domaininteraction.VideoStat) (*applicationinteraction.ActionStateResult, error) {
	actionType, err := domaininteraction.NormalizeActionType(actionType)
	if err != nil {
		return nil, err
	}
	key := interactionActionKey(userID, videoID, actionType)
	old, err := client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	oldActive := old["active"] == "1"
	delta := 0
	if old["idempotency_key"] == strings.TrimSpace(idempotencyKey) && idempotencyKey != "" {
		active = oldActive
	} else if old["active"] == "" {
		if active {
			delta = 1
		}
	} else if oldActive != active {
		if active {
			delta = 1
		} else {
			delta = -1
		}
	}
	baseStat := actionStatBaseInit(videoID, initial)
	baseKey := interactionStatCounterBaseKey(videoID)
	shardKey := interactionStatCounterShardKey(videoID, interactionStatCounterShardIndex(userID))
	_, err = client.TxPipelined(ctx, func(p redis.Pipeliner) error {
		v := "0"
		if active {
			v = "1"
		}
		p.HSet(ctx, key, "active", v, "idempotency_key", strings.TrimSpace(idempotencyKey))
		p.Expire(ctx, key, actionStateTTL)
		queueActionStatBaseInit(ctx, p, baseKey, baseStat)
		p.Expire(ctx, baseKey, actionStatTTL)
		if delta != 0 {
			p.HIncrBy(ctx, shardKey, interactionStatField(actionType), int64(delta))
		}
		p.Expire(ctx, shardKey, actionStatTTL)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &applicationinteraction.ActionStateResult{VideoID: videoID, ActionType: actionType, Active: active, Delta: delta, IdempotencyKey: strings.TrimSpace(idempotencyKey)}, nil
}

// redisActionStatReader 提供统计哈希和 JSON 的读取能力。
type redisActionStatReader interface {
	HGetAll(ctx context.Context, key string) *redis.MapStringStringCmd
	Get(ctx context.Context, key string) *redis.StringCmd
}

// redisActionStatWriter 提供聚合统计 JSON 的写入能力。
type redisActionStatWriter interface {
	Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
}

// redisStatCacheClient 组合批量统计读取与回填所需的缓存接口。
type redisStatCacheClient interface {
	redisActionStatReader
	redisActionStatWriter
	MGet(ctx context.Context, keys ...string) *redis.SliceCmd
}

const actionStatCounterShardCount = 16

const hotWindowMinutes = 60
const hotMinuteBucketTTL = 2 * time.Hour
const hotWindowCacheTTL = 2 * time.Minute
const actionStateTTL = 30 * 24 * time.Hour
const actionStatTTL = 24 * time.Hour
const actionStatJSONTTL = 15 * time.Second

// redisWatchCmdable 提供缓存读写和乐观事务。
type redisWatchCmdable interface {
	redis.Cmdable
	Pipeline() redis.Pipeliner
	Watch(context.Context, func(*redis.Tx) error, ...string) error
}

// FeedCache 使用 Redis 保存信息流和互动计数缓存。
type FeedCache struct{ client redisWatchCmdable }

// clampRedisCount 将 Redis 互动计数限制为不小于零。
func clampRedisCount(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

// NewFeedCache 创建 Feed 结果缓存。
// NewFeedCache 创建支持事务型互动状态更新的 Feed 缓存。
func NewFeedCache(client redisWatchCmdable) *FeedCache { return &FeedCache{client: client} }

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
	pipe := c.client.Pipeline()
	queued := false
	for id, v := range m {
		if v == nil || v.VideoID <= 0 {
			continue
		}
		b, e := json.Marshal(v)
		if e != nil {
			return e
		}
		pipe.Set(ctx, feedCardKey(id), b, ttl)
		queued = true
	}
	if !queued {
		return nil
	}
	_, e := pipe.Exec(ctx)
	return e
}

// GetStats 批量读取视频计数缓存。
func (c *FeedCache) GetStats(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error) {
	return getStats(ctx, c.client, videoIDs)
}

// getStats 优先读取 JSON，未命中时聚合分片并尽力回填缓存。
func getStats(ctx context.Context, client redisStatCacheClient, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error) {
	stats := map[int64]*domainfeed.FeedStat{}
	if len(videoIDs) == 0 {
		return stats, nil
	}

	values, err := client.MGet(ctx, cacheKeys(videoIDs, feedStatKey)...).Result()
	if err != nil {
		return nil, err
	}
	for index, value := range values {
		content, ok := cacheValueBytes(value)
		if !ok {
			continue
		}
		var stat domainfeed.FeedStat
		if err := json.Unmarshal(content, &stat); err != nil {
			continue
		}
		if stat.VideoID <= 0 {
			stat.VideoID = videoIDs[index]
		}
		stats[stat.VideoID] = &stat
	}
	for _, videoID := range videoIDs {
		if stats[videoID] != nil {
			continue
		}
		stat, ok, err := actionStatFromCache(ctx, client, videoID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		stats[videoID] = stat
		_ = setActionStatJSON(ctx, client, feedStatKey(videoID), stat)
	}
	return stats, nil
}

// SetStats 批量写入视频计数缓存。
func (c *FeedCache) SetStats(ctx context.Context, m map[int64]*domainfeed.FeedStat, ttl time.Duration) error {
	pipe := c.client.Pipeline()
	queued := false
	for id, v := range m {
		if v == nil || v.VideoID <= 0 {
			continue
		}
		b, e := json.Marshal(v)
		if e != nil {
			return e
		}
		pipe.Set(ctx, feedStatKey(id), b, ttl)
		queued = true
	}
	if !queued {
		return nil
	}
	_, e := pipe.Exec(ctx)
	return e
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
	exists, e := c.client.Exists(ctx, key).Result()
	if e != nil {
		return nil, e
	}
	if exists == 0 {
		if _, e = c.client.ZUnionStore(ctx, key, &redis.ZStore{Keys: hotWindowMinuteKeys(end), Aggregate: "SUM"}).Result(); e != nil {
			return nil, e
		}
		pipe := c.client.Pipeline()
		pipe.ZRemRangeByScore(ctx, key, "-inf", "0")
		pipe.Expire(ctx, key, hotWindowCacheTTL)
		if _, e = pipe.Exec(ctx); e != nil {
			return nil, e
		}
	}
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

// interactionActionKey 返回用户视频互动状态的 Redis 键。
func interactionActionKey(userID, videoID int64, actionType string) string {
	return fmt.Sprintf("interaction:action:v1:%d:%d:%s", userID, videoID, strings.ToLower(actionType))
}

// interactionStatCounterKey 返回视频互动计数 Hash 的 Redis 键。
func interactionStatCounterKey(videoID int64) string {
	return fmt.Sprintf("video:stat:counter:v1:%d", videoID)
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

// actionStat 聚合基数与分片，完全未命中时返回零计数。
func actionStat(ctx context.Context, client redisActionStatReader, counterBaseKey string, counterShardKeys []string, jsonKey string, videoID int64, initialStat *domaininteraction.VideoStat) (*domainfeed.FeedStat, error) {
	stat, _, err := actionStatWithPresence(ctx, client, counterBaseKey, counterShardKeys, jsonKey, videoID, initialStat)
	if err != nil {
		return nil, err
	}
	if stat == nil {
		return &domainfeed.FeedStat{VideoID: videoID}, nil
	}
	return stat, nil
}

// actionStatFromCache 读取缓存中的基数与分片并报告是否命中。
func actionStatFromCache(ctx context.Context, client redisActionStatReader, videoID int64) (*domainfeed.FeedStat, bool, error) {
	return actionStatWithPresence(ctx, client, interactionStatCounterBaseKey(videoID), interactionStatCounterShardKeys(videoID), feedStatKey(videoID), videoID, nil)
}

// actionStatWithPresence 优先读取基数，缺失时回退至 JSON 或初始统计，再叠加分片。
func actionStatWithPresence(ctx context.Context, client redisActionStatReader, counterBaseKey string, counterShardKeys []string, jsonKey string, videoID int64, initialStat *domaininteraction.VideoStat) (*domainfeed.FeedStat, bool, error) {
	stat := &domainfeed.FeedStat{VideoID: videoID}
	found := false
	values, err := client.HGetAll(ctx, counterBaseKey).Result()
	if err != nil {
		return nil, false, err
	}
	if len(values) > 0 {
		applyActionStatFields(stat, values)
		found = true
	} else {
		fallbackStat, ok, err := actionStatFallback(ctx, client, jsonKey, videoID, initialStat)
		if err != nil {
			return nil, false, err
		}
		if ok {
			stat = fallbackStat
			found = true
		}
	}
	shardFound, err := applyActionStatShardDeltas(ctx, client, stat, counterShardKeys)
	if err != nil {
		return nil, false, err
	}
	found = found || shardFound
	if !found {
		return nil, false, nil
	}
	return stat, true, nil
}

// actionStatFallback 读取 JSON 缓存，未命中时使用初始统计。
func actionStatFallback(ctx context.Context, client redisActionStatReader, jsonKey string, videoID int64, initialStat *domaininteraction.VideoStat) (*domainfeed.FeedStat, bool, error) {
	stat := &domainfeed.FeedStat{VideoID: videoID}
	content, err := client.Get(ctx, jsonKey).Bytes()
	if err == redis.Nil {
		if initialStat != nil {
			stat.LikeCount = initialStat.LikeCount
			stat.CommentCount = initialStat.CommentCount
			stat.FavoriteCount = initialStat.FavoriteCount
			return stat, true, nil
		}
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if err := json.Unmarshal(content, stat); err != nil {
		return nil, false, nil
	}
	if stat.VideoID <= 0 {
		stat.VideoID = videoID
	}
	return stat, true, nil
}

// actionStatBaseInit 返回传入统计作为基数，未提供时生成指定视频的零计数。
func actionStatBaseInit(videoID int64, initialStat *domaininteraction.VideoStat) *domaininteraction.VideoStat {
	if initialStat != nil {
		return initialStat
	}
	return &domaininteraction.VideoStat{VideoID: videoID}
}

// queueActionStatBaseInit 在事务管道中只初始化缺失的基数字段。
func queueActionStatBaseInit(ctx context.Context, pipe redis.Pipeliner, counterBaseKey string, initialStat *domaininteraction.VideoStat) {
	stat := &domaininteraction.VideoStat{}
	if initialStat != nil {
		stat = initialStat
	}
	pipe.HSetNX(ctx, counterBaseKey, "like_count", stat.LikeCount)
	pipe.HSetNX(ctx, counterBaseKey, "comment_count", stat.CommentCount)
	pipe.HSetNX(ctx, counterBaseKey, "favorite_count", stat.FavoriteCount)
}

// setActionStatJSON 序列化聚合计数并设置短期缓存。
func setActionStatJSON(ctx context.Context, client redisActionStatWriter, jsonKey string, stat *domainfeed.FeedStat) error {
	content, err := json.Marshal(stat)
	if err != nil {
		return err
	}
	return client.Set(ctx, jsonKey, content, actionStatJSONTTL).Err()
}

// applyActionStatShardDeltas 汇总点赞收藏增量并将最终计数限制为非负。
func applyActionStatShardDeltas(ctx context.Context, client redisActionStatReader, stat *domainfeed.FeedStat, shardKeys []string) (bool, error) {
	if stat == nil || len(shardKeys) == 0 {
		return false, nil
	}

	shardValues, err := loadActionStatShardValues(ctx, client, shardKeys)
	if err != nil {
		return false, err
	}
	found := false
	likeDelta := 0
	favoriteDelta := 0
	for _, values := range shardValues {
		if len(values) > 0 {
			found = true
		}
		likeDelta += actionStatFieldInt(values, "like_count")
		favoriteDelta += actionStatFieldInt(values, "favorite_count")
	}
	stat.LikeCount = clampRedisCount(stat.LikeCount + likeDelta)
	stat.FavoriteCount = clampRedisCount(stat.FavoriteCount + favoriteDelta)
	return found, nil
}

// loadActionStatShardValues 优先通过管道批量读分片，否则逐个读取并传递错误。
func loadActionStatShardValues(ctx context.Context, client redisActionStatReader, shardKeys []string) ([]map[string]string, error) {
	type pipelineProvider interface {
		Pipeline() redis.Pipeliner
	}

	if provider, ok := client.(pipelineProvider); ok {
		pipe := provider.Pipeline()
		cmds := make([]*redis.MapStringStringCmd, 0, len(shardKeys))
		for _, key := range shardKeys {
			cmds = append(cmds, pipe.HGetAll(ctx, key))
		}
		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			return nil, err
		}
		values := make([]map[string]string, 0, len(cmds))
		for _, cmd := range cmds {
			value, err := cmd.Result()
			if err != nil && err != redis.Nil {
				return nil, err
			}
			values = append(values, value)
		}
		return values, nil
	}

	values := make([]map[string]string, 0, len(shardKeys))
	for _, key := range shardKeys {
		value, err := client.HGetAll(ctx, key).Result()
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

// applyActionStatFields 将哈希字段转换为统计计数。
func applyActionStatFields(stat *domainfeed.FeedStat, values map[string]string) {
	if stat == nil {
		return
	}
	stat.LikeCount = actionStatFieldInt(values, "like_count")
	stat.CommentCount = actionStatFieldInt(values, "comment_count")
	stat.FavoriteCount = actionStatFieldInt(values, "favorite_count")
}

// actionStatFieldInt 解析计数字段，无法解析时返回零。
func actionStatFieldInt(values map[string]string, field string) int {
	value, _ := strconv.Atoi(values[field])
	return value
}

// interactionStatCounterBaseKey 返回视频计数基数键。
func interactionStatCounterBaseKey(videoID int64) string {
	return fmt.Sprintf("%s:base", interactionStatCounterKey(videoID))
}

// interactionStatCounterShardKey 返回指定视频的计数分片键。
func interactionStatCounterShardKey(videoID int64, shard int) string {
	return fmt.Sprintf("%s:shard:%02d", interactionStatCounterKey(videoID), shard)
}

// interactionStatCounterShardKeys 生成全部十六个计数分片键。
func interactionStatCounterShardKeys(videoID int64) []string {
	keys := make([]string, 0, actionStatCounterShardCount)
	for shard := 0; shard < actionStatCounterShardCount; shard++ {
		keys = append(keys, interactionStatCounterShardKey(videoID, shard))
	}
	return keys
}

// interactionStatCounterShardIndex 按用户编号选择分片，非正编号使用零号分片。
func interactionStatCounterShardIndex(userID int64) int {
	if userID <= 0 {
		return 0
	}
	return int(userID % actionStatCounterShardCount)
}

// interactionStatField 将互动类型映射为计数字段。
func interactionStatField(actionType string) string {
	if actionType == domaininteraction.ActionTypeLike {
		return "like_count"
	}
	return "favorite_count"
}
