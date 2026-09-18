package feed

import (
	"strings"
	"time"
)

const MaxLimit = 100

// BigCreatorFollowerThreshold 定义大作者采用关注流拉模式的粉丝数阈值。
const BigCreatorFollowerThreshold = 10000

// Scene 表示不同 Feed 场景，应用层通过场景选择对应策略。
type Scene string

const (
	SceneTimeline  Scene = "timeline"
	SceneRecommend Scene = "recommend"
	SceneFollowing Scene = "following"
	SceneHot       Scene = "hot"

	DefaultScene = SceneTimeline
)

// FeedItem 时间线视频项领域对象。
// 它包含了视频的基本信息，如视频ID、作者ID、标题、媒体URL、封面URL、点赞数、评论数、收藏数、发布时间等。
type FeedItem struct {
	VideoID         int64
	AuthorID        int64
	AuthorNickname  string
	AuthorAvatarURL string
	Title           string
	Description     string
	MediaURL        string
	CoverURL        string
	LikeCount       int
	CommentCount    int
	FavoriteCount   int
	HotScore        int
	PublishedAt     time.Time
}

// FeedPageItem 是 Feed 页缓存中的轻量条目，只保存排序和组装所需字段。
type FeedPageItem struct {
	AuthorID    int64
	VideoID     int64
	PublishedAt time.Time
	HotScore    int
}

// FeedCard 保存视频卡片中相对稳定的展示字段。
type FeedCard struct {
	VideoID         int64
	AuthorID        int64
	AuthorNickname  string
	AuthorAvatarURL string
	Title           string
	Description     string
	MediaURL        string
	CoverURL        string
}

// FeedStat 保存视频卡片中的高频计数字段。
type FeedStat struct {
	VideoID       int64
	LikeCount     int
	CommentCount  int
	FavoriteCount int
}

// TimelineCursor 时间游标，用于分页查询时间线。
// 它包含了视频的发布时间和视频ID，用于确定下一页的起始位置。
type TimelineCursor struct {
	PublishedAt time.Time
	VideoID     int64
}

// HotCursor 保存热榜分页所需的排序字段。
type HotCursor struct {
	HotScore    int
	PublishedAt time.Time
	VideoID     int64
	WindowEnd   time.Time
	Offset      int
}

// NormalizeScene 统一 scene 参数格式，空值使用默认 Feed 场景。
func NormalizeScene(scene Scene) Scene {
	value := strings.TrimSpace(strings.ToLower(string(scene)))
	if value == "" {
		return DefaultScene
	}
	return Scene(value)
}

// RestoreFeedItem 根据持久化数据恢复信息流条目领域对象。
func RestoreFeedItem(videoID int64, authorID int64, authorNickname string, authorAvatarURL string, title string, description string, mediaURL string, coverURL string, likeCount int, commentCount int, favoriteCount int, publishedAt time.Time) *FeedItem {
	return &FeedItem{
		VideoID:         videoID,
		AuthorID:        authorID,
		AuthorNickname:  strings.TrimSpace(authorNickname),
		AuthorAvatarURL: strings.TrimSpace(authorAvatarURL),
		Title:           strings.TrimSpace(title),
		Description:     strings.TrimSpace(description),
		MediaURL:        strings.TrimSpace(mediaURL),
		CoverURL:        strings.TrimSpace(coverURL),
		LikeCount:       likeCount,
		CommentCount:    commentCount,
		FavoriteCount:   favoriteCount,
		HotScore:        ScoreHotFeedItem(likeCount, commentCount, favoriteCount),
		PublishedAt:     publishedAt,
	}
}

// ScoreHotFeedItem 计算热榜排序分：评论权重最高，收藏次之，点赞提供基础热度。
func ScoreHotFeedItem(likeCount int, commentCount int, favoriteCount int) int {
	return likeCount*3 + commentCount*5 + favoriteCount*4
}
