package interaction

import domaininteraction "github.com/inumoroha/video-feed/internal/domain/interaction"

// ActionResult 表示点赞或收藏设置后的动作状态和相关统计。
type ActionResult struct {
	VideoID       int64
	ActionType    string
	Active        bool
	LikeCount     int
	FavoriteCount int
}

// CreateCommentResult 表示评论创建结果及视频当前评论数。
type CreateCommentResult struct {
	Comment      *domaininteraction.Comment
	CommentCount int
}

// DeleteCommentResult 表示评论删除结果及视频当前评论数。
type DeleteCommentResult struct {
	CommentID    int64
	Status       int
	CommentCount int
}

// CommentListResult 表示评论分页结果及下一页游标信息。
type CommentListResult struct {
	Items      []*domaininteraction.Comment
	NextCursor string
	HasMore    bool
}
