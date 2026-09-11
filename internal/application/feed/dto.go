package feed

import (
	"context"

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
	scene domainfeed.Scene
	repo  domainfeed.Repository
}

// HotStrategy 使用互动热度读取热榜 Feed。
type HotStrategy struct {
	repo domainfeed.Repository
}

// hotCursorPayload 表示热榜游标的编码载荷。
type hotCursorPayload struct {
	HotScore    int    `json:"hot_score"`
	PublishedAt string `json:"published_at"`
	VideoID     int64  `json:"video_id"`
}
