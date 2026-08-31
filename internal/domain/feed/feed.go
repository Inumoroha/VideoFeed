package feed

import (
	"strings"
	"time"
)

const (
	EventTypeImpression = "IMPRESSION"
	EventTypeView       = "VIEW"
	EventTypeComplete   = "COMPLETE"

	MaxLimit                = 100
	MaxVisitorIDLength      = 128
	MaxIdempotencyKeyLength = 128
	DefaultRefreshScene     = "time"
	TimeCursorScene         = "time"
)

// FeedItem 时间线视频项领域对象。
// 它包含了视频的基本信息，如视频ID、作者ID、标题、媒体URL、封面URL、点赞数、评论数、收藏数、发布时间等。
type FeedItem struct {
	VideoID       int64
	AuthorID      int64
	Title         string
	MediaURL      string
	CoverURL      string
	LikeCount     int
	CommentCount  int
	FavoriteCount int
	PublishedAt   time.Time
}

// TimeCursor 时间游标，用于分页查询时间线。
// 它包含了视频的发布时间和视频ID，用于确定下一页的起始位置。
type TimeCursor struct {
	PublishedAt time.Time
	VideoID     int64
}

// ViewEvent 观看事件领域对象。
// 它记录了用户或访客观看视频的事件，包括事件类型、观看时长、视频ID等。
type ViewEvent struct {
	ID             int64
	UserID         *int64
	VisitorID      string
	VideoID        int64
	EventType      string
	WatchMS        int
	IdempotencyKey string
	CreatedAt      time.Time
}

// NewViewEvent 校验观看事件参数并创建新的观看事件。
func NewViewEvent(userID *int64, visitorID string, videoID int64, eventType string, watchMS int, idempotencyKey string) (*ViewEvent, error) {
	visitorID = strings.TrimSpace(visitorID)
	eventType = strings.ToUpper(strings.TrimSpace(eventType))
	idempotencyKey = strings.TrimSpace(idempotencyKey)

	if userID != nil && *userID <= 0 {
		return nil, ErrInvalidUserID
	}
	if userID == nil && visitorID == "" {
		return nil, ErrMissingActor
	}
	if len(visitorID) > MaxVisitorIDLength {
		return nil, ErrVisitorIDTooLong
	}
	if videoID <= 0 {
		return nil, ErrInvalidVideoID
	}
	if eventType == "" {
		return nil, ErrEmptyEventType
	}
	if !isValidEventType(eventType) {
		return nil, ErrInvalidEventType
	}
	if watchMS < 0 {
		return nil, ErrInvalidWatchMS
	}
	if len(idempotencyKey) > MaxIdempotencyKeyLength {
		return nil, ErrIdempotencyKeyTooLong
	}

	return &ViewEvent{
		UserID:         userID,
		VisitorID:      visitorID,
		VideoID:        videoID,
		EventType:      eventType,
		WatchMS:        watchMS,
		IdempotencyKey: idempotencyKey,
	}, nil
}

// RestoreViewEvent 根据持久化数据恢复观看事件领域对象。
func RestoreViewEvent(id int64, userID *int64, visitorID string, videoID int64, eventType string, watchMS int, idempotencyKey string, createdAt time.Time) *ViewEvent {
	visitorID = strings.TrimSpace(visitorID)
	eventType = strings.ToUpper(strings.TrimSpace(eventType))
	idempotencyKey = strings.TrimSpace(idempotencyKey)

	return &ViewEvent{
		ID:             id,
		UserID:         userID,
		VisitorID:      visitorID,
		VideoID:        videoID,
		EventType:      eventType,
		WatchMS:        watchMS,
		IdempotencyKey: idempotencyKey,
		CreatedAt:      createdAt,
	}
}

// RestoreFeedItem 根据持久化数据恢复信息流条目领域对象。
func RestoreFeedItem(videoID int64, authorID int64, title string, mediaURL string, coverURL string, likeCount int, commentCount int, favoriteCount int, publishedAt time.Time) *FeedItem {
	return &FeedItem{
		VideoID:       videoID,
		AuthorID:      authorID,
		Title:         strings.TrimSpace(title),
		MediaURL:      strings.TrimSpace(mediaURL),
		CoverURL:      strings.TrimSpace(coverURL),
		LikeCount:     likeCount,
		CommentCount:  commentCount,
		FavoriteCount: favoriteCount,
		PublishedAt:   publishedAt,
	}
}

// Validate 校验时间游标；nil 游标表示从信息流首条记录开始查询。
func (c *TimeCursor) Validate() error {
	if c == nil {
		return nil
	}
	if c.PublishedAt.IsZero() || c.VideoID <= 0 {
		return ErrInvalidCursor
	}
	return nil
}

// isValidEventType 判断事件类型是否属于领域支持的枚举值。
func isValidEventType(value string) bool {
	switch value {
	case EventTypeImpression, EventTypeView, EventTypeComplete:
		return true
	default:
		return false
	}
}
