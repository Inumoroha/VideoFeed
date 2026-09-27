package interaction

import "context"

// Repository 定义互动动作和评论持久化所需的操作。
type Repository interface {
	SetAction(ctx context.Context, userID int64, videoID int64, actionType string, active bool, idempotencyKey string) (*Action, int, int, error)
	CreateComment(ctx context.Context, comment *Comment) (*Comment, int, error)
	FindCommentByUserAndIdempotencyKey(ctx context.Context, userID int64, idempotencyKey string) (*Comment, int, error)
	ListComments(ctx context.Context, videoID int64, cursor *CommentCursor, limit int) ([]*Comment, error)
	DeleteComment(ctx context.Context, commentID int64, userID int64, role string) (*Comment, int, error)
	// GetUserProfile 读取触发互动的用户展示资料，用于互动通知。
	GetUserProfile(ctx context.Context, userID int64) (*UserProfile, error)
}

// NotificationRepository 为通知提供视频作者和事务内实际变化量，重放返回零变化量。
type NotificationRepository interface {
	GetVideoAuthorID(context.Context, int64) (int64, error)
	SetActionWithDelta(context.Context, int64, int64, string, bool, string) (*Action, int, int, int, error)
	CreateCommentWithDelta(context.Context, *Comment) (*Comment, int, int, error)
}
