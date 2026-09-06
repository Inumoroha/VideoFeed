package interaction

import "time"

// createCommentRequest 表示创建评论请求体。
type createCommentRequest struct {
	Content string `json:"content"`
}

// actionResponse 表示点赞或收藏操作的响应数据。
type actionResponse struct {
	VideoID       int64  `json:"video_id"`
	ActionType    string `json:"action_type"`
	Active        bool   `json:"active"`
	LikeCount     int    `json:"like_count"`
	FavoriteCount int    `json:"favorite_count"`
}

// commentResponse 表示评论及其作者展示信息。
type commentResponse struct {
	ID            int64     `json:"id"`
	VideoID       int64     `json:"video_id"`
	UserID        int64     `json:"user_id"`
	UserNickname  string    `json:"user_nickname"`
	UserAvatarURL string    `json:"user_avatar_url"`
	Content       string    `json:"content"`
	CreatedAt     time.Time `json:"created_at"`
	CommentCount  int       `json:"comment_count,omitempty"`
}

// commentListResponse 表示评论游标分页响应。
type commentListResponse struct {
	Items      []commentResponse `json:"items"`
	NextCursor string            `json:"next_cursor"`
	HasMore    bool              `json:"has_more"`
}

// deleteCommentResponse 表示评论删除后的状态和统计。
type deleteCommentResponse struct {
	CommentID    int64 `json:"comment_id"`
	Status       int   `json:"status"`
	CommentCount int   `json:"comment_count"`
}
