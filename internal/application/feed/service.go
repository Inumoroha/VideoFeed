package feed

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
)

const defaultFeedLimit = 10
const timelineFirstPageCacheTTL = 5 * time.Second
const timelinePageCacheTTL = 45 * time.Second
const timelineLoadTimeout = 5 * time.Second

// Service 通过 scene 策略注册表分发不同 Feed 场景。
type Service struct {
	strategies   map[domainfeed.Scene]Strategy
	defaultScene domainfeed.Scene
}

// Option 用于在装配阶段注册额外 Feed 策略。
type Option func(*Service)

// WithStrategy 注册一个额外 Feed 策略。
func WithStrategy(strategy Strategy) Option {
	return func(s *Service) {
		s.RegisterStrategy(strategy)
	}
}

// WithTimelineCache 为时间线 Feed 启用读缓存。
func WithTimelineCache(cache FeedCache) Option {
	return func(s *Service) {
		strategy, ok := s.strategies[domainfeed.SceneTimeline].(*TimelineStrategy)
		if ok {
			strategy.cache = cache
		}
	}
}

// NewService 创建使用指定信息流仓储的应用服务。
func NewService(repo domainfeed.Repository, options ...Option) *Service {
	service := &Service{
		strategies:   map[domainfeed.Scene]Strategy{},
		defaultScene: domainfeed.DefaultScene,
	}
	service.RegisterStrategy(NewTimelineStrategy(domainfeed.SceneTimeline, repo))
	service.RegisterStrategy(NewHotStrategy(repo))
	for _, option := range options {
		option(service)
	}
	return service
}

// RegisterStrategy 把 scene 和具体策略绑定，新增 Feed 类型时在装配层调用。
func (s *Service) RegisterStrategy(strategy Strategy) {
	if strategy == nil {
		return
	}
	scene := domainfeed.NormalizeScene(strategy.Scene())
	s.strategies[scene] = strategy
}

// GetFeed 根据 scene 选择策略并返回分页结果。
func (s *Service) GetFeed(ctx context.Context, req FeedRequest) (*FeedResult, error) {
	if s == nil {
		return nil, ErrFeedServiceNotInitialized
	}
	req.Scene = domainfeed.NormalizeScene(req.Scene)
	if req.Scene == "" {
		req.Scene = s.defaultScene
	}

	strategy, ok := s.strategies[req.Scene]
	if !ok {
		return nil, domainfeed.ErrUnsupportedScene
	}
	return strategy.List(ctx, req)
}

// GetTimelineFeed 查询时间线视频流，处理游标分页并生成下一页游标。
func (s *Service) GetTimelineFeed(ctx context.Context, cursor string, limit int) (*TimelineFeedResult, error) {
	return s.GetFeed(ctx, FeedRequest{
		Scene:  domainfeed.SceneTimeline,
		Cursor: cursor,
		Limit:  limit,
	})
}

// NewTimelineStrategy 创建一个时间线排序策略，可作为 latest 等场景的基础实现。
func NewTimelineStrategy(scene domainfeed.Scene, repo domainfeed.Repository) *TimelineStrategy {
	return &TimelineStrategy{
		scene:        domainfeed.NormalizeScene(scene),
		repo:         repo,
		firstPageTTL: timelineFirstPageCacheTTL,
		pageTTL:      timelinePageCacheTTL,
	}
}

// Scene 返回当前策略绑定的 Feed 场景。
func (s *TimelineStrategy) Scene() domainfeed.Scene {
	return s.scene
}

// NewHotStrategy 创建热榜排序策略。
func NewHotStrategy(repo domainfeed.Repository) *HotStrategy {
	return &HotStrategy{repo: repo}
}

// Scene 返回热榜场景。
func (s *HotStrategy) Scene() domainfeed.Scene {
	return domainfeed.SceneHot
}

// List 使用热度分、发布时间和视频 ID 读取热榜 Feed。
func (s *HotStrategy) List(ctx context.Context, req FeedRequest) (*FeedResult, error) {
	if s == nil || s.repo == nil {
		return nil, ErrFeedServiceNotInitialized
	}
	if req.Limit < 0 {
		return nil, domainfeed.ErrInvalidLimit
	}
	parsedCursor, err := parseHotCursor(req.Cursor)
	if err != nil {
		return nil, err
	}
	limit := normalizeLimit(req.Limit)

	items, err := s.repo.ListHotFeed(ctx, parsedCursor, limit+1)
	if err != nil {
		return nil, ErrLoadFeedFailed
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	nextCursor := ""
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		nextCursor = encodeHotCursor(&domainfeed.HotCursor{
			HotScore:    last.HotScore,
			PublishedAt: last.PublishedAt,
			VideoID:     last.VideoID,
		})
	}

	return &FeedResult{
		Scene:      domainfeed.SceneHot,
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// RefreshTimelineFeed 从时间线起点查询视频流，适用于刷新首页数据。
func (s *Service) RefreshTimelineFeed(ctx context.Context, limit int) (*TimelineFeedResult, error) {
	return s.GetTimelineFeed(ctx, "", limit)
}

// List 分页读取时间线并按缓存键合并查询；共享查询独立限时，各请求可单独取消等待。
func (s *TimelineStrategy) List(ctx context.Context, req FeedRequest) (*FeedResult, error) {
	if s == nil || s.repo == nil {
		return nil, ErrFeedServiceNotInitialized
	}
	if req.Limit < 0 {
		return nil, domainfeed.ErrInvalidLimit
	}
	parsedCursor, err := parseTimelineCursor(req.Cursor)
	if err != nil {
		return nil, err
	}
	limit := normalizeLimit(req.Limit)

	if s.cache == nil {
		return s.listFromRepo(ctx, parsedCursor, limit)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	cacheKey := timelineCacheKey(req.Cursor, limit)
	if result, ok, err := s.cache.Get(ctx, cacheKey); err == nil && ok {
		return result, nil
	}

	resultCh := s.group.DoChan(cacheKey, func() (any, error) {
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timelineLoadTimeout)
		defer cancel()
		if result, ok, err := s.cache.Get(loadCtx, cacheKey); err == nil && ok {
			return result, nil
		}
		result, err := s.listFromRepo(loadCtx, parsedCursor, limit)
		if err != nil {
			return nil, err
		}
		_ = s.cache.Set(loadCtx, cacheKey, result, timelineCacheTTL(req.Cursor, cacheKey, s.firstPageTTL, s.pageTTL))
		return result, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case shared := <-resultCh:
		if shared.Err != nil {
			return nil, shared.Err
		}
		result, ok := shared.Val.(*FeedResult)
		if !ok {
			return nil, ErrLoadFeedFailed
		}
		return result, nil
	}
}

// listFromRepo 多查一条时间线记录以判断是否还有下一页，并生成游标。
func (s *TimelineStrategy) listFromRepo(ctx context.Context, parsedCursor *domainfeed.TimelineCursor, limit int) (*FeedResult, error) {
	items, err := s.repo.ListTimelineFeed(ctx, parsedCursor, limit+1)
	if err != nil {
		return nil, ErrLoadFeedFailed
	}

	// limit+1 是常见分页技巧：多取一条即可判断后面还有没有数据。
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	nextCursor := ""
	if hasMore && len(items) > 0 {
		// 下一页从当前页最后一个元素之后开始，游标保存排序所需的两个字段。
		nextCursor = encodeTimelineCursor(&domainfeed.TimelineCursor{
			PublishedAt: items[len(items)-1].PublishedAt,
			VideoID:     items[len(items)-1].VideoID,
		})
	}

	return &FeedResult{
		Scene:      s.scene,
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// normalizeLimit 将零值分页数量转换为默认值，并限制最大返回数量。
func normalizeLimit(limit int) int {
	if limit == 0 {
		return defaultFeedLimit
	}
	if limit > domainfeed.MaxLimit {
		return domainfeed.MaxLimit
	}
	return limit
}

// timelineCacheKey 根据游标和分页数量生成时间线缓存键。
func timelineCacheKey(cursor string, limit int) string {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return fmt.Sprintf("feed:timeline:v1:limit:%d:first", limit)
	}

	sum := sha1.Sum([]byte(cursor))
	return fmt.Sprintf("feed:timeline:v1:limit:%d:cursor:%s", limit, hex.EncodeToString(sum[:]))
}

// timelineCacheTTL 区分首页与后续页，为正有效期增加由缓存键决定的百分之十至二十的偏移。
func timelineCacheTTL(cursor string, cacheKey string, firstPageTTL time.Duration, pageTTL time.Duration) time.Duration {
	ttl := pageTTL
	if strings.TrimSpace(cursor) == "" {
		ttl = firstPageTTL
	}
	if ttl <= 0 {
		return 0
	}

	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(cacheKey))
	jitterPercent := 10 + int(hasher.Sum32()%11)
	return ttl + time.Duration(jitterPercent)*ttl/100
}

// parseTimelineCursor 解码并校验时间线分页游标。
func parseTimelineCursor(raw string) (*domainfeed.TimelineCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	content, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		content, err = base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, domainfeed.ErrInvalidCursor
		}
	}

	var payload timelineCursorPayload
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, domainfeed.ErrInvalidCursor
	}

	publishedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(payload.PublishedAt))
	if err != nil || publishedAt.IsZero() || payload.VideoID <= 0 {
		return nil, domainfeed.ErrInvalidCursor
	}

	return &domainfeed.TimelineCursor{
		PublishedAt: publishedAt,
		VideoID:     payload.VideoID,
	}, nil
}

// parseHotCursor 将客户端传回的热榜游标解析成领域游标。
func parseHotCursor(raw string) (*domainfeed.HotCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	content, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		content, err = base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, domainfeed.ErrInvalidCursor
		}
	}

	var payload hotCursorPayload
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, domainfeed.ErrInvalidCursor
	}

	publishedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(payload.PublishedAt))
	if err != nil || publishedAt.IsZero() || payload.VideoID <= 0 {
		return nil, domainfeed.ErrInvalidCursor
	}

	return &domainfeed.HotCursor{
		HotScore:    payload.HotScore,
		PublishedAt: publishedAt,
		VideoID:     payload.VideoID,
	}, nil
}

// encodeTimelineCursor 将时间线分页游标编码为 URL 安全字符串。
func encodeTimelineCursor(cursor *domainfeed.TimelineCursor) string {
	if cursor == nil || cursor.VideoID <= 0 || cursor.PublishedAt.IsZero() {
		return ""
	}

	content, err := json.Marshal(timelineCursorPayload{
		PublishedAt: cursor.PublishedAt.UTC().Format(time.RFC3339Nano),
		VideoID:     cursor.VideoID,
	})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(content)
}

// encodeHotCursor 把热榜排序字段编码成 URL 安全的游标字符串。
func encodeHotCursor(cursor *domainfeed.HotCursor) string {
	if cursor == nil || cursor.VideoID <= 0 || cursor.PublishedAt.IsZero() {
		return ""
	}

	content, err := json.Marshal(hotCursorPayload{
		HotScore:    cursor.HotScore,
		PublishedAt: cursor.PublishedAt.UTC().Format(time.RFC3339Nano),
		VideoID:     cursor.VideoID,
	})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(content)
}
