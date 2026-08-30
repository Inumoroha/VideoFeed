package feed

import (
	"context"
	"errors"
	"strings"
	"time"

	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

var _ domainfeed.Repository = (*Repository)(nil)

type timeFeedVideoModel struct {
	VideoID       int64     `gorm:"column:video_id"`
	AuthorID      int64     `gorm:"column:author_id"`
	Title         string    `gorm:"column:title"`
	MediaURL      string    `gorm:"column:media_url"`
	CoverURL      string    `gorm:"column:cover_url"`
	LikeCount     int       `gorm:"column:like_count"`
	CommentCount  int       `gorm:"column:comment_count"`
	FavoriteCount int       `gorm:"column:favorite_count"`
	PublishedAt   time.Time `gorm:"column:published_at"`
}

// New 创建基于 GORM 数据库实例的信息流仓储。
func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ListTimeFeed 按发布时间和视频 ID 倒序分页查询已发布视频流。
func (r *Repository) ListTimeFeed(ctx context.Context, cursor *domainfeed.TimeCursor, limit int) ([]*domainfeed.FeedItem, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("feed repository is not initialized")
	}
	if limit <= 0 {
		return nil, domainfeed.ErrInvalidLimit
	}
	if limit > domainfeed.MaxLimit {
		limit = domainfeed.MaxLimit
	}
	if err := cursor.Validate(); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var models []timeFeedVideoModel
	query := r.db.WithContext(ctx).
		Table("videos").
		Select("id AS video_id, author_id, title, media_url, cover_url, like_count, comment_count, favorite_count, published_at").
		Where("status = ? AND published_at IS NOT NULL", domainvideo.StatusPublished)

	if cursor != nil {
		query = query.Where(
			"(published_at < ? OR (published_at = ? AND id < ?))",
			cursor.PublishedAt,
			cursor.PublishedAt,
			cursor.VideoID,
		)
	}

	err := query.
		Order("published_at DESC").
		Order("id DESC").
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
			model.Title,
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

// SaveViewEvent 保存观看事件，并将数据库生成的 ID 和创建时间回写到领域对象。
func (r *Repository) SaveViewEvent(ctx context.Context, event *domainfeed.ViewEvent) error {
	if r == nil || r.db == nil {
		return errors.New("feed repository is not initialized")
	}
	if event == nil {
		return errors.New("view event is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	model := FeedViewEventModel{
		UserID:         event.UserID,
		VisitorID:      event.VisitorID,
		VideoID:        event.VideoID,
		EventType:      event.EventType,
		WatchMS:        event.WatchMS,
		IdempotencyKey: idempotencyKeyPtr(event.IdempotencyKey),
	}

	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		if isDuplicateKeyError(err) {
			return domainfeed.ErrDuplicateIdempotencyKey
		}
		return err
	}

	event.ID = model.ID
	event.CreatedAt = model.CreatedAt
	return nil
}

// FindViewEventByIdempotencyKey 按幂等键查询已保存的观看事件。
func (r *Repository) FindViewEventByIdempotencyKey(ctx context.Context, key string) (*domainfeed.ViewEvent, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("feed repository is not initialized")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, domainfeed.ErrViewEventNotFound
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var model FeedViewEventModel
	err := r.db.WithContext(ctx).
		Where("idempotency_key = ?", key).
		First(&model).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainfeed.ErrViewEventNotFound
		}
		return nil, err
	}
	return restoreViewEvent(model), nil
}

// restoreViewEvent 将观看事件持久化模型转换为领域对象。
func restoreViewEvent(model FeedViewEventModel) *domainfeed.ViewEvent {
	return domainfeed.RestoreViewEvent(
		model.ID,
		model.UserID,
		model.VisitorID,
		model.VideoID,
		model.EventType,
		model.WatchMS,
		idempotencyKeyValue(model.IdempotencyKey),
		model.CreatedAt,
	)
}

// idempotencyKeyPtr 将空幂等键转换为 nil，避免占用唯一索引值。
func idempotencyKeyPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// idempotencyKeyValue 将数据库中的可空幂等键转换为空字符串。
func idempotencyKeyValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// isDuplicateKeyError 判断错误是否为 GORM 翻译后的唯一键冲突。
func isDuplicateKeyError(err error) bool {
	return errors.Is(err, gorm.ErrDuplicatedKey)
}
