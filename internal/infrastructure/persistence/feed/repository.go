package feed

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"
	"gorm.io/gorm"
)

const hotScoreExpression = "COALESCE(vs.like_count, 0) * 3 + COALESCE(vs.comment_count, 0) * 5 + COALESCE(vs.favorite_count, 0) * 4"

// Repository 使用 GORM 持久化信息流数据。
type Repository struct {
	db *gorm.DB
}

var _ domainfeed.Repository = (*Repository)(nil)

// timelineFeedVideoModel 表示时间线查询所需的视频及作者字段。
type timelineFeedVideoModel struct {
	VideoID         int64     `gorm:"column:video_id"`
	AuthorID        int64     `gorm:"column:author_id"`
	AuthorNickname  string    `gorm:"column:author_nickname"`
	AuthorAvatarURL string    `gorm:"column:author_avatar_url"`
	Title           string    `gorm:"column:title"`
	Description     string    `gorm:"column:description"`
	MediaURL        string    `gorm:"column:media_url"`
	CoverURL        string    `gorm:"column:cover_url"`
	LikeCount       int       `gorm:"column:like_count"`
	CommentCount    int       `gorm:"column:comment_count"`
	FavoriteCount   int       `gorm:"column:favorite_count"`
	HotScore        int       `gorm:"column:hot_score"`
	PublishedAt     time.Time `gorm:"column:published_at"`
}

// New 创建基于 GORM 数据库实例的信息流仓储。
func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// EnsureTimelineIndex 在 videos 表上创建 timeline 回源查询所需的复合索引，索引已存在时直接跳过。
func EnsureTimelineIndex(db *gorm.DB) error {
	if db == nil {
		return errors.New("gorm database is nil")
	}

	return db.Exec(
		"CREATE INDEX IF NOT EXISTS idx_video_timeline ON videos (status, published_at DESC, id DESC)",
	).Error
}

// ListTimelineFeed 按发布时间和视频 ID 倒序分页查询已发布视频流。
func (r *Repository) ListTimelineFeed(ctx context.Context, cursor *domainfeed.TimelineCursor, limit int) ([]*domainfeed.FeedItem, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("feed repository is not initialized")
	}
	if limit <= 0 {
		return nil, domainfeed.ErrInvalidLimit
	}
	if limit > domainfeed.MaxLimit+1 {
		limit = domainfeed.MaxLimit + 1
	}
	if cursor != nil && (cursor.PublishedAt.IsZero() || cursor.VideoID <= 0) {
		return nil, domainfeed.ErrInvalidCursor
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var models []timelineFeedVideoModel
	// query := r.db.WithContext(ctx).
	// 	Table("videos AS v").
	// 	Select("v.id AS video_id, v.author_id, COALESCE(u.nickname, '') AS author_nickname, COALESCE(u.avatar_url, '') AS author_avatar_url, v.title, v.description, v.media_url, v.cover_url, COALESCE(vs.like_count, 0) AS like_count, COALESCE(vs.comment_count, 0) AS comment_count, COALESCE(vs.favorite_count, 0) AS favorite_count, v.published_at").
	// 	Joins("LEFT JOIN users AS u ON u.id = v.author_id").
	// 	Joins("LEFT JOIN video_stat AS vs ON vs.video_id = v.id").
	// 	Where("v.status = ? AND v.published_at IS NOT NULL", domainvideo.StatusPublished)
	query := r.baseFeedQuery(ctx)

	if cursor != nil {
		query = query.Where(
			"(v.published_at < ? OR (v.published_at = ? AND v.id < ?))",
			cursor.PublishedAt,
			cursor.PublishedAt,
			cursor.VideoID,
		)
	}

	err := query.
		Order("v.published_at DESC").
		Order("v.id DESC").
		Limit(limit).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}

	items := make([]*domainfeed.FeedItem, 0, len(models))
	for _, model := range models {
		items = append(items, domainfeed.RestoreFeedItem(
			model.VideoID,
			model.AuthorID,
			model.AuthorNickname,
			model.AuthorAvatarURL,
			model.Title,
			model.Description,
			model.MediaURL,
			model.CoverURL,
			model.LikeCount,
			model.CommentCount,
			model.FavoriteCount,
			model.PublishedAt,
		))
	}
	return items, nil
}

// ListHotFeed 查询热榜 Feed，按互动热度倒序，并使用发布时间和视频 ID 保持稳定排序。
func (r *Repository) ListHotFeed(ctx context.Context, cursor *domainfeed.HotCursor, limit int) ([]*domainfeed.FeedItem, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("feed repository is not initialized")
	}
	if limit <= 0 {
		return nil, domainfeed.ErrInvalidLimit
	}
	if limit > domainfeed.MaxLimit+1 {
		limit = domainfeed.MaxLimit + 1
	}
	if cursor != nil && (cursor.PublishedAt.IsZero() || cursor.VideoID <= 0) {
		return nil, domainfeed.ErrInvalidCursor
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var models []timelineFeedVideoModel
	query := r.baseFeedQuery(ctx)

	if cursor != nil {
		query = query.Where(
			fmt.Sprintf("((%[1]s) < ? OR ((%[1]s) = ? AND v.published_at < ?) OR ((%[1]s) = ? AND v.published_at = ? AND v.id < ?))", hotScoreExpression),
			cursor.HotScore,
			cursor.HotScore,
			cursor.PublishedAt,
			cursor.HotScore,
			cursor.PublishedAt,
			cursor.VideoID,
		)
	}

	err := query.
		Order("hot_score DESC").
		Order("v.published_at DESC").
		Order("v.id DESC").
		Limit(limit).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}

	return feedItemsFromModels(models), nil
}

func (r *Repository) baseFeedQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("videos AS v").
		Select("v.id AS video_id, v.author_id, a.nickname AS author_nickname, a.avatar_url AS author_avatar_url, v.title, v.description, v.media_url, v.cover_url, COALESCE(vs.like_count, 0) AS like_count, COALESCE(vs.comment_count, 0) AS comment_count, COALESCE(vs.favorite_count, 0) AS favorite_count, ("+hotScoreExpression+") AS hot_score, v.published_at").
		Joins("LEFT JOIN users AS a ON a.id = v.author_id").
		Joins("LEFT JOIN video_stat AS vs ON vs.video_id = v.id").
		Where("v.status = ? AND v.published_at IS NOT NULL", domainvideo.StatusPublished)
}

func feedItemsFromModels(models []timelineFeedVideoModel) []*domainfeed.FeedItem {
	items := make([]*domainfeed.FeedItem, 0, len(models))
	for _, model := range models {
		// 仓储层把数据库查询结果恢复成领域 FeedItem，HTTP 层只处理响应格式。
		item := domainfeed.RestoreFeedItem(
			model.VideoID,
			model.AuthorID,
			model.AuthorNickname,
			model.AuthorAvatarURL,
			model.Title,
			model.Description,
			model.MediaURL,
			model.CoverURL,
			model.LikeCount,
			model.CommentCount,
			model.FavoriteCount,
			model.PublishedAt,
		)
		item.HotScore = model.HotScore
		items = append(items, item)
	}
	return items
}
