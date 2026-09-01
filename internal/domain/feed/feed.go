package feed

import (
	"strings"
	"time"
)

const MaxLimit = 100



// FeedItem 时间线视频项领域对象。
// 它包含了视频的基本信息，如视频ID、作者ID、标题、媒体URL、封面URL、点赞数、评论数、收藏数、发布时间等。
type FeedItem struct {
	VideoID       	int64
	AuthorID      	int64
	AuthorNickname 	string
	AuthorAvatarURL string
	Title         	string
	Description   	string
	MediaURL      	string
	CoverURL     	 string
	LikeCount     	int
	CommentCount  	int
	FavoriteCount 	int
	PublishedAt   	time.Time
}

// TimeCursor 时间游标，用于分页查询时间线。
// 它包含了视频的发布时间和视频ID，用于确定下一页的起始位置。
type TimelineCursor struct {
	PublishedAt time.Time
	VideoID     int64
}

// RestoreFeedItem 根据持久化数据恢复信息流条目领域对象。
func RestoreFeedItem(videoID int64, authorID int64, authorNickname string, authorAvatarURL string, title string, description string, mediaURL string, coverURL string, likeCount int, commentCount int, favoriteCount int, publishedAt time.Time) *FeedItem {
	return &FeedItem{
		VideoID:       		videoID,
		AuthorID:      		authorID,
		AuthorNickname: 	strings.TrimSpace(authorNickname),
		AuthorAvatarURL: 	strings.TrimSpace(authorAvatarURL),
		Title:         		strings.TrimSpace(title),
		Description:   		strings.TrimSpace(description),
		MediaURL:      		strings.TrimSpace(mediaURL),
		CoverURL:      		strings.TrimSpace(coverURL),
		LikeCount:     		likeCount,
		CommentCount:  		commentCount,
		FavoriteCount: 		favoriteCount,
		PublishedAt:   		publishedAt,
	}
}
