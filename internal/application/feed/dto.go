package feed

import (
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
)

type TimeFeedResult struct {
	Items      []*domainfeed.FeedItem
	NextCursor string
	HasMore    bool
}

type ViewEventResult struct {
	Event   *domainfeed.ViewEvent
	Created bool
}

type timeCursorPayload struct {
	PublishedAt string `json:"published_at"`
	VideoID     int64  `json:"video_id"`
}
