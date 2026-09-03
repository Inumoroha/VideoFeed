package interaction

import "time"

// ActionModel 表示 interaction_action 表中的用户互动动作记录。
type ActionModel struct {
	ID             int64     `gorm:"column:id;primaryKey;autoIncrement"`
	UserID         int64     `gorm:"column:user_id;not null;uniqueIndex:uk_user_video_type,priority:1;index:idx_user_type_status,priority:1"`
	VideoID        int64     `gorm:"column:video_id;not null;uniqueIndex:uk_user_video_type,priority:2;index:idx_video_type_status,priority:1"`
	ActionType     string    `gorm:"column:action_type;size:16;not null;uniqueIndex:uk_user_video_type,priority:3;index:idx_video_type_status,priority:2;index:idx_user_type_status,priority:2"`
	Status         int       `gorm:"column:status;not null;default:1;index:idx_video_type_status,priority:3;index:idx_user_type_status,priority:3"`
	IdempotencyKey *string   `gorm:"column:idempotency_key;size:128"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName 返回互动动作持久化记录对应的数据表名称。
func (ActionModel) TableName() string {
	return "interaction_action"
}

// ActionIdempotencyModel 保存点赞或收藏请求的首次执行结果，用于幂等重试。
type ActionIdempotencyModel struct {
	ID             int64     `gorm:"column:id;primaryKey;autoIncrement"`
	UserID         int64     `gorm:"column:user_id;not null;uniqueIndex:uk_user_action_idempotency,priority:1"`
	ActionType     string    `gorm:"column:action_type;size:16;not null;uniqueIndex:uk_user_action_idempotency,priority:2"`
	IdempotencyKey string    `gorm:"column:idempotency_key;size:128;not null;uniqueIndex:uk_user_action_idempotency,priority:3"`
	VideoID        int64     `gorm:"column:video_id;not null"`
	Active         bool      `gorm:"column:active;not null"`
	LikeCount      int       `gorm:"column:like_count;not null"`
	FavoriteCount  int       `gorm:"column:favorite_count;not null"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
}

// TableName 返回互动动作幂等记录对应的数据表名称。
func (ActionIdempotencyModel) TableName() string {
	return "interaction_action_idempotency"
}

// CommentModel 表示 interaction_comment 表中的评论记录。
type CommentModel struct {
	ID             int64     `gorm:"column:id;primaryKey;autoIncrement"`
	VideoID        int64     `gorm:"column:video_id;not null;index:idx_video_status_created,priority:1"`
	UserID         int64     `gorm:"column:user_id;not null;index:idx_user_created,priority:1;uniqueIndex:uk_user_idempotency,priority:1"`
	Content        string    `gorm:"column:content;size:1000;not null"`
	Status         int       `gorm:"column:status;not null;default:1;index:idx_video_status_created,priority:2"`
	IdempotencyKey *string   `gorm:"column:idempotency_key;size:128;uniqueIndex:uk_user_idempotency,priority:2"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime;index:idx_video_status_created,priority:3;index:idx_user_created,priority:2"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime;index:idx_video_status_created,priority:4"`
}

// TableName 返回评论持久化记录对应的数据表名称。
func (CommentModel) TableName() string {
	return "interaction_comment"
}
