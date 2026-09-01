package feed

import (
	"context"
	"errors"
	"time"

	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"

	"gorm.io/gorm"
)

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
	PublishedAt     time.Time `gorm:"column:published_at"`
}

// New 创建基于 GORM 数据库实例的信息流仓储。
func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
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
	query := r.db.WithContext(ctx).
		Table("videos AS v").
		Select("v.id AS video_id, v.author_id, COALESCE(u.nickname, '') AS author_nickname, COALESCE(u.avatar_url, '') AS author_avatar_url, v.title, v.description, v.media_url, v.cover_url, v.like_count, v.comment_count, v.favorite_count, v.published_at").
		Joins("LEFT JOIN users AS u ON u.id = v.author_id").
		Where("v.status = ? AND v.published_at IS NOT NULL", domainvideo.StatusPublished)

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
