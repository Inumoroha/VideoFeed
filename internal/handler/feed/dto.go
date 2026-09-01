package feed

import "time"

// timelineFeedResponse 表示时间线查询分页接口返回的结果。
type timelineFeedResponse struct {
	Items      []feedItemResponse `json:"items"`
	NextCursor string             `json:"next_cursor"`
	HasMore    bool               `json:"has_more"`
}

// feedItemResponse 表示信息流中的单条视频数据。
type feedItemResponse struct {
	VideoID         int64     `json:"video_id"`
	AuthorID        int64     `json:"author_id"`
	AuthorNickname  string    `json:"author_nickname"`
	AuthorAvatarURL string    `json:"author_avatar_url"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	MediaURL        string    `json:"media_url"`
	CoverURL        string    `json:"cover_url"`
	LikeCount       int       `json:"like_count"`
	CommentCount    int       `json:"comment_count"`
	FavoriteCount   int       `json:"favorite_count"`
	PublishedAt     time.Time `json:"published_at"`
}
