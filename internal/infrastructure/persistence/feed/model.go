package feed

import "time"

// FeedCursorModel 表示服务端保存的信息流分页游标及其有效期。
type FeedCursorModel struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement"`
	Scene       string    `gorm:"column:scene;size:32;not null;index:idx_scene_expired,priority:1"`
	CursorToken string    `gorm:"column:cursor_token;size:128;not null;uniqueIndex:uk_cursor_token"`
	PublishedAt time.Time `gorm:"column:published_at;not null"`
	VideoID     int64     `gorm:"column:video_id;not null"`
	ExpiredAt   time.Time `gorm:"column:expired_at;not null;index:idx_scene_expired,priority:2"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
}

// TableName 返回分页游标对应的数据表名称。
func (FeedCursorModel) TableName() string {
	return "feed_cursor"
}

// FeedViewEventModel 表示用户或访客的视频观看事件持久化记录。
type FeedViewEventModel struct {
	ID             int64     `gorm:"column:id;primaryKey;autoIncrement"`
	UserID         *int64    `gorm:"column:user_id;index:idx_user_time,priority:1"`
	VisitorID      string    `gorm:"column:visitor_id;size:128;index:idx_visitor_time,priority:1"`
	VideoID        int64     `gorm:"column:video_id;not null;index:idx_video_time,priority:1"`
	EventType      string    `gorm:"column:event_type;size:16;not null"`
	WatchMS        int       `gorm:"column:watch_ms;not null;default:0"`
	IdempotencyKey *string   `gorm:"column:idempotency_key;size:128;uniqueIndex:uk_idempotency_key"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime;index:idx_user_time,priority:2;index:idx_video_time,priority:2;index:idx_visitor_time,priority:2"`
}

// TableName 返回观看事件对应的数据表名称。
func (FeedViewEventModel) TableName() string {
	return "feed_view_event"
}
