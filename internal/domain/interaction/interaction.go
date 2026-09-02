package interaction

import (
	"strings"
	"time"
	"unicode/utf8"
)

const (
	ActionTypeLike     = "LIKE"
	ActionTypeFavorite = "FAVORITE"

	ActionStatusActive   = 1
	ActionStatusCanceled = 2

	CommentStatusNormal  = 1
	CommentStatusDeleted = 2

	MaxCommentContentLength = 1000
	MaxIdempotencyKeyLength = 128
	MaxLimit                = 100
)

// Action 表示用户对视频执行的点赞或收藏动作及其当前状态。
type Action struct {
	ID             int64
	UserID         int64
	VideoID        int64
	ActionType     string
	Status         int
	IdempotencyKey string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Comment 表示用户发布的视频评论及其作者展示信息。
type Comment struct {
	ID             int64
	VideoID        int64
	UserID         int64
	UserNickname   string
	UserAvatarURL  string
	Content        string
	Status         int
	IdempotencyKey string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// CommentCursor 表示评论列表分页使用的时间和 ID 游标。
type CommentCursor struct {
	CreatedAt time.Time
	CommentID int64
}

// NormalizeActionType 清理并校验动作类型，返回统一的大写值。
func NormalizeActionType(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value != ActionTypeLike && value != ActionTypeFavorite {
		return "", ErrInvalidActionType
	}
	return value, nil
}

// NewComment 校验评论参数并创建一条正常状态的评论。
func NewComment(videoID int64, userID int64, content string, idempotencyKey string) (*Comment, error) {
	if videoID <= 0 {
		return nil, ErrInvalidVideoID
	}
	if userID <= 0 {
		return nil, ErrInvalidUserID
	}

	content = strings.TrimSpace(content)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if content == "" {
		return nil, ErrEmptyCommentContent
	}
	if utf8.RuneCountInString(content) > MaxCommentContentLength {
		return nil, ErrCommentContentTooLong
	}
	if len(idempotencyKey) > MaxIdempotencyKeyLength {
		return nil, ErrIdempotencyKeyTooLong
	}

	return &Comment{
		VideoID:        videoID,
		UserID:         userID,
		Content:        content,
		Status:         CommentStatusNormal,
		IdempotencyKey: idempotencyKey,
	}, nil
}

// RestoreAction 根据持久化字段恢复动作，遇到非法动作类型时返回错误。
func RestoreAction(id int64, userID int64, videoID int64, actionType string, status int, idempotencyKey string, createdAt time.Time, updatedAt time.Time) (*Action, error) {
	var err error
	actionType, err = NormalizeActionType(actionType)
	if err != nil {
		return nil, err
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if status == 0 {
		status = ActionStatusActive
	}

	return &Action{
		ID:             id,
		UserID:         userID,
		VideoID:        videoID,
		ActionType:     actionType,
		Status:         status,
		IdempotencyKey: idempotencyKey,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}, nil
}

// RestoreComment 根据持久化字段恢复评论，并清理可展示的文本字段。
func RestoreComment(id int64, videoID int64, userID int64, userNickname string, userAvatarURL string, content string, status int, idempotencyKey string, createdAt time.Time, updatedAt time.Time) *Comment {
	content = strings.TrimSpace(content)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if status == 0 {
		status = CommentStatusNormal
	}

	return &Comment{
		ID:             id,
		VideoID:        videoID,
		UserID:         userID,
		UserNickname:   strings.TrimSpace(userNickname),
		UserAvatarURL:  strings.TrimSpace(userAvatarURL),
		Content:        content,
		Status:         status,
		IdempotencyKey: idempotencyKey,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}
}

// Active 判断动作是否处于生效状态。
func (a *Action) Active() bool {
	return a != nil && a.Status == ActionStatusActive
}

// Deleted 判断评论是否处于已删除状态。
func (c *Comment) Deleted() bool {
	return c != nil && c.Status == CommentStatusDeleted
}
