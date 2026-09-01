package feed

import (
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
)

// TimelineFeedResult 表示时间线查询结果及下一页游标。
type TimelineFeedResult struct {
	Items      []*domainfeed.FeedItem
	NextCursor string
	HasMore    bool
}

// timelineCursorPayload 表示时间线游标的编码载荷。
type timelineCursorPayload struct {
	PublishedAt string `json:"published_at"`
	VideoID     int64  `json:"video_id"`
}
