package infravideo

import (
	"context"
	"errors"
	"time"

	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	domainrelation "github.com/inumoroha/video-feed/internal/domain/relation"
	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"
	"gorm.io/gorm"
)

// Repository 使用 GORM 持久化视频及其统计数据。
type Repository struct {
	db *gorm.DB
}

// videoWithStatModel 表示关联视频内容和互动统计后的查询结果。
type videoWithStatModel struct {
	ID             int64
	AuthorID       int64
	Title          string
	Description    string
	MediaURL       string
	CoverURL       string
	Status         int
	LikeCount      int
	CommentCount   int
	FavoriteCount  int
	PublishedAt    *time.Time
	IdempotencyKey *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

var _ domainvideo.Repository = (*Repository)(nil)

// New 创建基于 GORM 数据库实例的视频仓储。
func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// EnsureStats 为没有统计记录的视频补齐默认统计。
func EnsureStats(db *gorm.DB) error {
	if db == nil {
		return errors.New("gorm database is nil")
	}

	query := "INSERT INTO video_stat (video_id, like_count, comment_count, favorite_count, created_at, updated_at) " +
		"SELECT v.id, 0, 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP FROM videos AS v " +
		"LEFT JOIN video_stat AS vs ON vs.video_id = v.id " +
		"WHERE vs.video_id IS NULL " +
		"ON CONFLICT (video_id) DO NOTHING"
	return db.Exec(query).Error
}

// Save 在同一事务中创建视频、初始统计和普通作者的粉丝收件箱，任一步失败均回滚。
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

	var model VideoModel
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		model = VideoModel{
			AuthorID:       video.AuthorID,
			Title:          video.Title,
			Description:    video.Description,
			MediaURL:       video.MediaURL,
			CoverURL:       video.CoverURL,
			Status:         video.Status,
			PublishedAt:    video.PublishedAt,
			IdempotencyKey: idempotencyKeyPtr(video.IdempotencyKey),
		}
		if err := tx.Create(&model).Error; err != nil {
			if isDuplicateKeyError(err) {
				return domainvideo.ErrDuplicateIdempotencyKey
			}
			return err
		}

		stat := VideoStatModel{
			VideoID:       model.ID,
			LikeCount:     video.LikeCount,
			CommentCount:  video.CommentCount,
			FavoriteCount: video.FavoriteCount,
		}
		if err := tx.Create(&stat).Error; err != nil {
			return err
		}
		return fanoutPublishedVideoToFollowers(tx, model.AuthorID, model.ID, model.PublishedAt)
	})
	if err != nil {
		return err
	}

	video.ID = model.ID
	video.CreatedAt = model.CreatedAt
	video.UpdatedAt = model.UpdatedAt
	return nil
}

// authorFollowerStat 承载作者粉丝数查询结果。
type authorFollowerStat struct {
	FollowerCount int
}

// fanoutPublishedVideoToFollowers 在调用方事务内向普通作者的有效粉丝写入收件箱，重复条目原子更新发布时间；大作者跳过推送。
func fanoutPublishedVideoToFollowers(tx *gorm.DB, authorID int64, videoID int64, publishedAt *time.Time) error {
	if publishedAt == nil {
		return nil
	}

	followerCount, err := authorFollowerCount(tx, authorID)
	if err != nil {
		return err
	}
	if followerCount >= domainfeed.BigCreatorFollowerThreshold {
		return nil
	}

	return tx.Exec(`
		INSERT INTO feed_inbox (user_id, video_id, author_id, published_at, created_at)
		SELECT f.user_id, ?, ?, ?, NOW()
		FROM user_follow AS f
		WHERE f.target_user_id = ? AND f.status = ?
		ON CONFLICT (user_id, video_id) DO UPDATE SET published_at = EXCLUDED.published_at
	`, videoID, authorID, *publishedAt, authorID, domainrelation.FollowStatusActive).Error
}

// authorFollowerCount 在当前事务内读取作者粉丝数，缺失统计按零处理，其他错误交由调用方回滚。
func authorFollowerCount(tx *gorm.DB, authorID int64) (int, error) {
	var stat authorFollowerStat
	err := tx.Table("user_relation_stat").
		Select("follower_count").
		Where("user_id = ?", authorID).
		Take(&stat).
		Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	return stat.FollowerCount, err
}

// FindByID 查询指定的已发布视频，并合并其互动统计数据。
func (r *Repository) FindByID(ctx context.Context, id int64) (*domainvideo.Video, error) {
	if err := r.validateQuery(id); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var model videoWithStatModel
	err := r.videoWithStatQuery(ctx).
		Where("v.id = ? AND v.status = ?", id, domainvideo.StatusPublished).
		Take(&model).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainvideo.ErrVideoNotFound
		}
		return nil, err
	}
	return restoreVideo(model), nil
}

// FindByIDAnyStatus 查询指定视频，不限制其当前状态，并合并互动统计数据。
func (r *Repository) FindByIDAnyStatus(ctx context.Context, id int64) (*domainvideo.Video, error) {
	if err := r.validateQuery(id); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var model videoWithStatModel
	err := r.videoWithStatQuery(ctx).
		Where("v.id = ?", id).
		Take(&model).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainvideo.ErrVideoNotFound
		}
		return nil, err
	}
	return restoreVideo(model), nil
}

// FindByAuthorAndIdempotencyKey 按作者和幂等键查询已创建的视频，并合并互动统计数据。
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

	var model videoWithStatModel
	err := r.videoWithStatQuery(ctx).
		Where("v.author_id = ? AND v.idempotency_key = ?", authorID, key).
		Take(&model).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainvideo.ErrVideoNotFound
		}
		return nil, err
	}
	return restoreVideo(model), nil
}

// ListByAuthor 按发布时间倒序分页查询作者已发布的视频及其互动统计数据。
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

	var models []videoWithStatModel
	err := r.videoWithStatQuery(ctx).
		Where("v.author_id = ? AND v.status = ?", authorID, domainvideo.StatusPublished).
		Order("v.published_at DESC").
		Order("v.id DESC").
		Limit(limit).
		Offset(offset).
		Scan(&models).
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

// validateQuery 校验视频查询使用的仓储实例和视频 ID。
func (r *Repository) validateQuery(id int64) error {
	if r == nil || r.db == nil {
		return errors.New("video repository is not initialized")
	}
	if id <= 0 {
		return domainvideo.ErrInvalidVideoID
	}
	return nil
}

// videoWithStatQuery 创建关联 videos 和 video_stat 的基础查询。
func (r *Repository) videoWithStatQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("videos AS v").
		Select(videoWithStatSelect()).
		Joins("LEFT JOIN video_stat AS vs ON vs.video_id = v.id")
}

// restoreVideo 将关联查询结果转换为视频领域对象。
func restoreVideo(model videoWithStatModel) *domainvideo.Video {
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

// videoWithStatSelect 返回视频及统计字段的统一查询列清单。
func videoWithStatSelect() string {
	return "v.id, v.author_id, v.title, v.description, v.media_url, v.cover_url, v.status, " +
		"COALESCE(vs.like_count, 0) AS like_count, " +
		"COALESCE(vs.comment_count, 0) AS comment_count, " +
		"COALESCE(vs.favorite_count, 0) AS favorite_count, " +
		"v.published_at, v.idempotency_key, v.created_at, v.updated_at"
}

// idempotencyKeyPtr 将空幂等键转换为 nil，避免占用唯一约束。
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

// isDuplicateKeyError 判断错误是否为 GORM 转换后的唯一键冲突。
func isDuplicateKeyError(err error) bool {
	return errors.Is(err, gorm.ErrDuplicatedKey)
}
