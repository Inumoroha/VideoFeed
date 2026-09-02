package infravideo

import "time"

// VideoModel 表示 videos 表中的视频持久化记录。
type VideoModel struct {
	ID             int64      `gorm:"column:id;primaryKey;autoIncrement"`
	AuthorID       int64      `gorm:"column:author_id;not null;index:idx_author_status,priority:1;uniqueIndex:uk_author_idempotency,priority:1"`
	Title          string     `gorm:"column:title;size:128;not null"`
	Description    string     `gorm:"column:description;size:512;not null;default:''"`
	MediaURL       string     `gorm:"column:media_url;size:512;not null"`
	CoverURL       string     `gorm:"column:cover_url;size:512;not null"`
	Status         int        `gorm:"column:status;not null;default:2;index:idx_author_status,priority:2;index:idx_status_published,priority:1"`
	PublishedAt    *time.Time `gorm:"column:published_at;index:idx_status_published,priority:2"`
	IdempotencyKey *string    `gorm:"column:idempotency_key;size:128;uniqueIndex:uk_author_idempotency,priority:2"`
	CreatedAt      time.Time  `gorm:"column:created_at;autoCreateTime;index:idx_author_status,priority:3"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName 返回视频持久化记录对应的数据表名称。
func (VideoModel) TableName() string {
	return "videos"
}

// VideoStatModel 表示 video_stat 表中的视频统计持久化记录。
type VideoStatModel struct {
	VideoID       int64     `gorm:"column:video_id;primaryKey;not null"`
	LikeCount     int       `gorm:"column:like_count;not null;default:0"`
	CommentCount  int       `gorm:"column:comment_count;not null;default:0"`
	FavoriteCount int       `gorm:"column:favorite_count;not null;default:0"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName 返回视频统计持久化记录对应的数据表名称。
func (VideoStatModel) TableName() string {
	return "video_stat"
}
