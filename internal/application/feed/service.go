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
	"golang.org/x/sync/singleflight"
)

const defaultFeedLimit = 10
const timelineFirstPageCacheTTL = 5 * time.Second
const timelinePageCacheTTL = 45 * time.Second
const timelineLoadTimeout = 5 * time.Second
const hotFirstPageCacheTTL = 10 * time.Second
const hotPageCacheTTL = 30 * time.Second
const feedCardCacheTTL = 15 * time.Minute
const feedStatCacheTTL = 15 * time.Second


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

// WithFeedCache 为 Feed 页、卡片和计数启用读缓存。
func WithFeedCache(cache FeedCache) Option {
	return func(s *Service) {
		for _, strategy := range s.strategies {
			switch typed := strategy.(type) {
			case *TimelineStrategy:
				typed.cache = cache
			case *HotStrategy:
				typed.cache = cache
			}
		}
	}
}

// WithTimelineCache 保留原有装配入口，为时间线 Feed 启用读缓存。
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
	return &HotStrategy{
		repo:         repo,
		firstPageTTL: hotFirstPageCacheTTL,
		pageTTL:      hotPageCacheTTL,
	}
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

	page, err := loadFeedPage(ctx, s.cache, domainfeed.SceneHot, req.Cursor, limit, s.firstPageTTL, s.pageTTL, &s.group, func() (*FeedPage, error) {
		return s.listPageFromRepo(ctx, parsedCursor, limit)
	})
	if err != nil {
		return nil, err
	}
	items, err := assembleFeedItems(ctx, s.repo, s.cache, page.Items)
	if err != nil {
		return nil, ErrLoadFeedFailed
	}

	return &FeedResult{
		Scene:      domainfeed.SceneHot,
		Items:      items,
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
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

	page, err := loadFeedPage(ctx, s.cache, s.scene, req.Cursor, limit, s.firstPageTTL, s.pageTTL, &s.group, func() (*FeedPage, error) {
		return s.listPageFromRepo(ctx, parsedCursor, limit)
	})
	if err != nil {
		return nil, err
	}

	items, err := assembleFeedItems(ctx, s.repo, s.cache, page.Items)
	if err != nil {
		return nil, ErrLoadFeedFailed
	}

	return &FeedResult{
		Scene:      s.scene,
		Items:      items,
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	}, nil
	
}

// listFromRepo 多查一条热榜记录以判断是否还有下一页，并生成游标。
func (s *HotStrategy) listPageFromRepo(ctx context.Context, parsedCursor *domainfeed.HotCursor, limit int) (*FeedPage, error) {
	items, err := s.repo.ListHotPage(ctx, parsedCursor, limit+1)
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

	return &FeedPage{
		Scene:      domainfeed.SceneHot,
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

func loadFeedPage(ctx context.Context, cache FeedCache, scene domainfeed.Scene, cursor string, limit int, firstPageTTL time.Duration, pageTTL time.Duration, group *singleflight.Group, load func() (*FeedPage, error)) (*FeedPage, error) {
	if cache == nil || group == nil {
		return load()
	}

	cacheKey := feedPageCacheKey(scene, cursor, limit)
	if page, ok, err := cache.GetPage(ctx, cacheKey); err == nil && ok {
		return page, nil
	}

	value, err, _ := group.Do(cacheKey, func() (any, error) {
		if page, ok, err := cache.GetPage(ctx, cacheKey); err == nil && ok {
			return page, nil
		}
		page, err := load()
		if err != nil {
			return nil, err
		}
		_ = cache.SetPage(ctx, cacheKey, page, feedPageCacheTTL(cursor, cacheKey, firstPageTTL, pageTTL))
		return page, nil
	})
	if err != nil {
		return nil, err
	}
	page, ok := value.(*FeedPage)
	if !ok {
		return nil, ErrLoadFeedFailed
	}
	return page, nil
}

func assembleFeedItems(ctx context.Context, repo domainfeed.Repository, cache FeedCache, pageItems []*domainfeed.FeedPageItem) ([]*domainfeed.FeedItem, error) {
	videoIDs := feedPageVideoIDs(pageItems)
	if len(videoIDs) == 0 {
		return []*domainfeed.FeedItem{}, nil
	}

	cards := map[int64]*domainfeed.FeedCard{}
	stats := map[int64]*domainfeed.FeedStat{}
	if cache != nil {
		if cachedCards, err := cache.GetCards(ctx, videoIDs); err == nil {
			cards = cachedCards
		}
		if cachedStats, err := cache.GetStats(ctx, videoIDs); err == nil {
			stats = cachedStats
		}
	}

	missingCardIDs := missingCardIDs(videoIDs, cards)
	if len(missingCardIDs) > 0 {
		loadedCards, err := repo.BatchGetFeedCards(ctx, missingCardIDs)
		if err != nil {
			return nil, err
		}
		mergeCards(cards, loadedCards)
		if cache != nil {
			_ = cache.SetCards(ctx, loadedCards, feedCardCacheTTL)
		}
	}

	missingStatIDs := missingStatIDs(videoIDs, stats)
	if len(missingStatIDs) > 0 {
		loadedStats, err := repo.BatchGetFeedStats(ctx, missingStatIDs)
		if err != nil {
			return nil, err
		}
		mergeStats(stats, loadedStats)
		if cache != nil {
			_ = cache.SetStats(ctx, loadedStats, feedStatCacheTTL)
		}
	}

	items := make([]*domainfeed.FeedItem, 0, len(pageItems))
	for _, pageItem := range pageItems {
		if pageItem == nil {
			continue
		}
		card, ok := cards[pageItem.VideoID]
		if !ok || card == nil {
			continue
		}
		stat := stats[pageItem.VideoID]
		if stat == nil {
			stat = &domainfeed.FeedStat{VideoID: pageItem.VideoID}
		}
		item := domainfeed.RestoreFeedItem(
			card.VideoID,
			card.AuthorID,
			card.AuthorNickname,
			card.AuthorAvatarURL,
			card.Title,
			card.Description,
			card.MediaURL,
			card.CoverURL,
			stat.LikeCount,
			stat.CommentCount,
			stat.FavoriteCount,
			pageItem.PublishedAt,
		)
		item.HotScore = pageItem.HotScore
		items = append(items, item)
	}
	return items, nil
}

func feedPageVideoIDs(items []*domainfeed.FeedPageItem) []int64 {
	videoIDs := make([]int64, 0, len(items))
	seen := map[int64]struct{}{}
	for _, item := range items {
		if item == nil || item.VideoID <= 0 {
			continue
		}
		if _, ok := seen[item.VideoID]; ok {
			continue
		}
		seen[item.VideoID] = struct{}{}
		videoIDs = append(videoIDs, item.VideoID)
	}
	return videoIDs
}

func missingCardIDs(videoIDs []int64, cards map[int64]*domainfeed.FeedCard) []int64 {
	missing := make([]int64, 0)
	for _, videoID := range videoIDs {
		if cards[videoID] == nil {
			missing = append(missing, videoID)
		}
	}
	return missing
}

func missingStatIDs(videoIDs []int64, stats map[int64]*domainfeed.FeedStat) []int64 {
	missing := make([]int64, 0)
	for _, videoID := range videoIDs {
		if stats[videoID] == nil {
			missing = append(missing, videoID)
		}
	}
	return missing
}

func mergeCards(target map[int64]*domainfeed.FeedCard, source map[int64]*domainfeed.FeedCard) {
	for videoID, card := range source {
		if card != nil {
			target[videoID] = card
		}
	}
}

func mergeStats(target map[int64]*domainfeed.FeedStat, source map[int64]*domainfeed.FeedStat) {
	for videoID, stat := range source {
		if stat != nil {
			target[videoID] = stat
		}
	}
}
// timelineCacheKey 根据游标和分页数量生成时间线缓存键。
func feedPageCacheKey(scene domainfeed.Scene, cursor string, limit int) string {
	scene = domainfeed.NormalizeScene(scene)
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return fmt.Sprintf("feed:page:v1:%s:limit:%d:first", scene, limit)
	}

	sum := sha1.Sum([]byte(cursor))
	return fmt.Sprintf("feed:page:v1:%s:limit:%d:cursor:%s", scene, limit, hex.EncodeToString(sum[:]))
}

// timelineCacheTTL 区分首页与后续页，为正有效期增加由缓存键决定的百分之十至二十的偏移。
func feedPageCacheTTL(cursor string, cacheKey string, firstPageTTL time.Duration, pageTTL time.Duration) time.Duration {

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
