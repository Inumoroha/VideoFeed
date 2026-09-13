package feed

import (
	"context"
	"time"

	"golang.org/x/sync/singleflight"

	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
)

// FeedResult 是游标分页结果，NextCursor 供客户端请求下一页。
type FeedResult struct {
	Scene      domainfeed.Scene
	Items      []*domainfeed.FeedItem
	NextCursor string
	HasMore    bool
}

// TimelineFeedResult 兼容原有时间线用例命名。
type TimelineFeedResult = FeedResult

// timelineCursorPayload 表示时间线游标的编码载荷。
type timelineCursorPayload struct {
	PublishedAt string `json:"published_at"`
	VideoID     int64  `json:"video_id"`
}

// FeedRequest 是所有 Feed 场景共用的查询参数。
type FeedRequest struct {
	Scene         domainfeed.Scene
	Cursor        string
	Limit         int
	ViewerID      int64
	ClientContext map[string]string
}

// Strategy 定义单个 Feed 场景的读取策略。
type Strategy interface {
	Scene() domainfeed.Scene
	List(ctx context.Context, req FeedRequest) (*FeedResult, error)
}

// TimelineStrategy 复用现有时间线查询能力。
type TimelineStrategy struct {
	scene        domainfeed.Scene
	repo         domainfeed.Repository
	cache        FeedCache
	firstPageTTL time.Duration
	pageTTL      time.Duration
	// group 按缓存键合并正在执行的查询，与用户身份无关。
	group singleflight.Group
}

// HotStrategy 使用互动热度读取热榜 Feed。
type HotStrategy struct {
	repo         domainfeed.Repository
	cache        FeedCache
	firstPageTTL time.Duration
	pageTTL      time.Duration
	group        singleflight.Group
}
// hotCursorPayload 表示热榜游标的编码载荷。
type hotCursorPayload struct {
	HotScore    int    `json:"hot_score"`
	PublishedAt string `json:"published_at"`
	VideoID     int64  `json:"video_id"`
}
// FeedCache 定义 Feed 页、卡片和计数缓存能力，Redis 实现在基础设施层提供。
type FeedCache interface {
	GetPage(ctx context.Context, key string) (*FeedPage, bool, error)
	SetPage(ctx context.Context, key string, page *FeedPage, ttl time.Duration) error
	GetCards(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedCard, error)
	SetCards(ctx context.Context, cards map[int64]*domainfeed.FeedCard, ttl time.Duration) error
	GetStats(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error)
	SetStats(ctx context.Context, stats map[int64]*domainfeed.FeedStat, ttl time.Duration) error
}

// FeedPage 是页缓存中的轻量结果，卡片和计数会在组装阶段批量读取。
type FeedPage struct {
	Scene      domainfeed.Scene
	Items      []*domainfeed.FeedPageItem
	NextCursor string
	HasMore    bool
}
