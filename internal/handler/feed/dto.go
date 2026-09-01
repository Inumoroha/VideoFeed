package feed

import "time"

// viewEventRequest 表示观看事件上报接口接收的请求字段。
type viewEventRequest struct {
	VisitorID string `json:"visitor_id"`
	VideoID   int64  `json:"video_id"`
	EventType string `json:"event_type"`
	WatchMS   int    `json:"watch_ms"`
}

// timeFeedResponse 表示信息流分页接口返回的结果。
type timeFeedResponse struct {
	Items      []feedItemResponse `json:"items"`
	NextCursor string             `json:"next_cursor"`
	HasMore    bool               `json:"has_more"`
}

// feedItemResponse 表示信息流中的单条视频数据。
type feedItemResponse struct {
	VideoID       int64     `json:"video_id"`
	AuthorID      int64     `json:"author_id"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	MediaURL      string    `json:"media_url"`
	CoverURL      string    `json:"cover_url"`
	LikeCount     int       `json:"like_count"`
	CommentCount  int       `json:"comment_count"`
	FavoriteCount int       `json:"favorite_count"`
	PublishedAt   time.Time `json:"published_at"`
}

// viewEventResponse 表示观看事件接口返回的事件数据。
type viewEventResponse struct {
	ID        int64     `json:"id"`
	VisitorID string    `json:"visitor_id,omitempty"`
	VideoID   int64     `json:"video_id"`
	EventType string    `json:"event_type"`
	WatchMS   int       `json:"watch_ms"`
	CreatedAt time.Time `json:"created_at"`
}
