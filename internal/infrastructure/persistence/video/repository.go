package infravideo

import (
	"context"
	"errors"

	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

var _ domainvideo.Repository = (*Repository)(nil)

// New 创建基于 GORM 数据库实例的视频仓储。
func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Save 新增视频并回写数据库生成的标识与时间字段。
func (r *Repository) Save(ctx context.Context, video *domainvideo.Video) error {
	if r == nil || r.db == nil {
		return errors.New("video repository is not initialized")
	}
	if video == nil {
		return errors.New("video is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	model := VideoModel{
		AuthorID:       video.AuthorID,
		Title:          video.Title,
		Description:    video.Description,
		MediaURL:       video.MediaURL,
		CoverURL:       video.CoverURL,
		Status:         video.Status,
		LikeCount:      video.LikeCount,
		CommentCount:   video.CommentCount,
		FavoriteCount:  video.FavoriteCount,
		PublishedAt:    video.PublishedAt,
		IdempotencyKey: idempotencyKeyPtr(video.IdempotencyKey),
	}

	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		if isDuplicateKeyError(err) {
			return domainvideo.ErrDuplicateIdempotencyKey
		}
		return err
	}

	video.ID = model.ID
	video.CreatedAt = model.CreatedAt
	video.UpdatedAt = model.UpdatedAt
	return nil
}

// FindByID 查询指定已发布视频，并将持久化记录恢复为领域对象。
func (r *Repository) FindByID(ctx context.Context, id int64) (*domainvideo.Video, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("video repository is not initialized")
	}
	if id <= 0 {
		return nil, domainvideo.ErrInvalidVideoID
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var model VideoModel
	err := r.db.WithContext(ctx).
		Where("id = ? AND status = ?", id, domainvideo.StatusPublished).
		First(&model).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainvideo.ErrVideoNotFound
		}
		return nil, err
	}
	return restoreVideo(model), nil
}

// FindByIDAnyStatus 查询指定视频，不限制其当前状态。
func (r *Repository) FindByIDAnyStatus(ctx context.Context, id int64) (*domainvideo.Video, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("video repository is not initialized")
	}
	if id <= 0 {
		return nil, domainvideo.ErrInvalidVideoID
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var model VideoModel
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&model).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainvideo.ErrVideoNotFound
		}
		return nil, err
	}
	return restoreVideo(model), nil
}

// FindByAuthorAndIdempotencyKey 按作者和幂等键查询已创建的视频。
func (r *Repository) FindByAuthorAndIdempotencyKey(ctx context.Context, authorID int64, key string) (*domainvideo.Video, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("video repository is not initialized")
	}
	if authorID <= 0 {
		return nil, domainvideo.ErrInvalidAuthorID
	}
	if key == "" {
		return nil, domainvideo.ErrVideoNotFound
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var model VideoModel
	err := r.db.WithContext(ctx).
		Where("author_id = ? AND idempotency_key = ?", authorID, key).
		First(&model).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainvideo.ErrVideoNotFound
		}
		return nil, err
	}
	return restoreVideo(model), nil
}

// ListByAuthor 按发布时间倒序分页查询作者已发布的视频。
func (r *Repository) ListByAuthor(ctx context.Context, authorID int64, limit, offset int) ([]*domainvideo.Video, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("video repository is not initialized")
	}
	if authorID <= 0 {
		return nil, domainvideo.ErrInvalidAuthorID
	}
	if limit <= 0 {
		return nil, domainvideo.ErrInvalidLimit
	}
	if offset < 0 {
		return nil, domainvideo.ErrInvalidOffset
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var models []VideoModel
	err := r.db.WithContext(ctx).
		Where("author_id = ? AND status = ?", authorID, domainvideo.StatusPublished).
		Order("published_at DESC").
		Order("id DESC").
		Limit(limit).
		Offset(offset).
		Find(&models).
		Error
	if err != nil {
		return nil, err
	}

	videos := make([]*domainvideo.Video, 0, len(models))
	for _, model := range models {
		videos = append(videos, restoreVideo(model))
	}
	return videos, nil
}

// UpdateStatus 更新指定视频的状态，并在记录不存在时返回领域错误。
func (r *Repository) UpdateStatus(ctx context.Context, video *domainvideo.Video) error {
	if r == nil || r.db == nil {
		return errors.New("video repository is not initialized")
	}
	if video == nil {
		return errors.New("video is nil")
	}
	if video.ID <= 0 {
		return domainvideo.ErrInvalidVideoID
	}
	if ctx == nil {
		ctx = context.Background()
	}

	result := r.db.WithContext(ctx).
		Model(&VideoModel{}).
		Where("id = ?", video.ID).
		Update("status", video.Status)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domainvideo.ErrVideoNotFound
	}
	return nil
}

// restoreVideo 将视频持久化记录转换为领域对象。
func restoreVideo(model VideoModel) *domainvideo.Video {
	return domainvideo.RestoreVideo(
		model.ID,
		model.AuthorID,
		model.Title,
		model.Description,
		model.MediaURL,
		model.CoverURL,
		model.Status,
		model.LikeCount,
		model.CommentCount,
		model.FavoriteCount,
		model.PublishedAt,
		model.CreatedAt,
		model.UpdatedAt,
		idempotencyKeyValue(model.IdempotencyKey),
	)
}

// idempotencyKeyPtr 将空幂等键转换为 nil，以避免占用唯一约束。
func idempotencyKeyPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// idempotencyKeyValue 将数据库中的可空幂等键转换为字符串。
func idempotencyKeyValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// isDuplicateKeyError 判断错误是否为 GORM 已转换的唯一键冲突。
func isDuplicateKeyError(err error) bool {
	return errors.Is(err, gorm.ErrDuplicatedKey)
}
