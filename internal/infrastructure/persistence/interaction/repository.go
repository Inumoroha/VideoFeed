package interaction

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	domaininteraction "github.com/inumoroha/video-feed/internal/domain/interaction"
	domainuser "github.com/inumoroha/video-feed/internal/domain/user"
	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"
	infravideo "github.com/inumoroha/video-feed/internal/infrastructure/persistence/video"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository 使用 GORM 持久化互动动作、评论和视频统计。
type Repository struct {
	db *gorm.DB
}

// commentWithUserModel 表示关联评论和用户展示信息后的查询结果。
type commentWithUserModel struct {
	ID             int64     `gorm:"column:id"`
	VideoID        int64     `gorm:"column:video_id"`
	UserID         int64     `gorm:"column:user_id"`
	UserNickname   string    `gorm:"column:user_nickname"`
	UserAvatarURL  string    `gorm:"column:user_avatar_url"`
	Content        string    `gorm:"column:content"`
	Status         int       `gorm:"column:status"`
	IdempotencyKey *string   `gorm:"column:idempotency_key"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

// New 创建基于 GORM 数据库实例的互动仓储。
func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// GetVideoStat 读取视频当前互动计数。
func (r *Repository) GetVideoStat(ctx context.Context, videoID int64) (*domaininteraction.VideoStat, error) {
	var stat infravideo.VideoStatModel
	err := r.db.WithContext(ctx).Table("video_stat").Where("video_id = ?", videoID).Take(&stat).Error
	if err != nil {
		return nil, mapVideoError(err)
	}
	return &domaininteraction.VideoStat{VideoID: stat.VideoID, LikeCount: stat.LikeCount, CommentCount: stat.CommentCount, FavoriteCount: stat.FavoriteCount}, nil
}

// SetAction 设置用户对已发布视频的点赞或收藏状态，并持久化首次幂等结果和统计快照。
func (r *Repository) SetAction(ctx context.Context, userID int64, videoID int64, actionType string, active bool, idempotencyKey string) (*domaininteraction.Action, int, int, error) {
	return r.setAction(ctx, userID, videoID, actionType, active, idempotencyKey, nil)
}

// SetActionWithDelta 保留互动写入行为，并返回事务内变化量；幂等重放或失败时为零。
func (r *Repository) SetActionWithDelta(ctx context.Context, userID int64, videoID int64, actionType string, active bool, idempotencyKey string) (*domaininteraction.Action, int, int, int, error) {
	delta := 0
	action, likes, favorites, err := r.setAction(ctx, userID, videoID, actionType, active, idempotencyKey, &delta)
	if err != nil {
		delta = 0
	}
	return action, likes, favorites, delta, err
}

// setAction 在原有事务和唯一约束下写入互动，可选返回成功状态转换的变化量。
func (r *Repository) setAction(ctx context.Context, userID int64, videoID int64, actionType string, active bool, idempotencyKey string, changed *int) (*domaininteraction.Action, int, int, error) {
	if err := r.validateRepository(); err != nil {
		return nil, 0, 0, err
	}
	if userID <= 0 {
		return nil, 0, 0, domaininteraction.ErrInvalidUserID
	}
	if videoID <= 0 {
		return nil, 0, 0, domaininteraction.ErrInvalidVideoID
	}
	var err error
	actionType, err = domaininteraction.NormalizeActionType(actionType)
	if err != nil {
		return nil, 0, 0, err
	}
	if len(strings.TrimSpace(idempotencyKey)) > domaininteraction.MaxIdempotencyKeyLength {
		return nil, 0, 0, domaininteraction.ErrIdempotencyKeyTooLong
	}
	if ctx == nil {
		ctx = context.Background()
	}

	idempotencyKey = strings.TrimSpace(idempotencyKey)
	var action ActionModel
	var likeCount int
	var favoriteCount int
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if idempotencyKey != "" {
			var idempotency ActionIdempotencyModel
			lookupErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("user_id = ? AND action_type = ? AND idempotency_key = ?", userID, actionType, idempotencyKey).
				Take(&idempotency).Error
			if lookupErr == nil {
				action = ActionModel{
					UserID:         userID,
					VideoID:        idempotency.VideoID,
					ActionType:     actionType,
					Status:         actionStatusFromActive(idempotency.Active),
					IdempotencyKey: idempotencyKeyPtr(idempotencyKey),
				}
				likeCount = idempotency.LikeCount
				favoriteCount = idempotency.FavoriteCount
				return nil
			}
			if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				return lookupErr
			}
		}

		if err := lockPublishedVideo(tx, videoID); err != nil {
			return err
		}

		findErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND video_id = ? AND action_type = ?", userID, videoID, actionType).
			Take(&action).Error
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}

		delta := 1
		nextStatus := actionStatusFromActive(active)
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			action = ActionModel{
				UserID:         userID,
				VideoID:        videoID,
				ActionType:     actionType,
				Status:         domaininteraction.ActionStatusActive,
				IdempotencyKey: idempotencyKeyPtr(idempotencyKey),
			}
			if err := tx.Create(&action).Error; err != nil {
				return err
			}
		} else {
			if action.Status == nextStatus {
				action.Status = domaininteraction.ActionStatusCanceled
				delta = -1
			} else {
				action.Status = domaininteraction.ActionStatusActive
			}
			action.IdempotencyKey = idempotencyKeyPtr(idempotencyKey)
			if err := tx.Save(&action).Error; err != nil {
				return err
			}
		}

		if changed != nil {
			*changed = delta
		}
		if _, err = updateActionStat(tx, videoID, actionType, delta); err != nil {
			return err
		}
		var stat infravideo.VideoStatModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("video_id = ?", videoID).Take(&stat).Error; err != nil {
			return err
		}
		likeCount = stat.LikeCount
		favoriteCount = stat.FavoriteCount
		if idempotencyKey == "" {
			return nil
		}
		return tx.Create(&ActionIdempotencyModel{
			UserID:         userID,
			ActionType:     actionType,
			IdempotencyKey: idempotencyKey,
			VideoID:        action.VideoID,
			Active:         action.Status == domaininteraction.ActionStatusActive,
			LikeCount:      likeCount,
			FavoriteCount:  favoriteCount,
		}).Error
	})
	if err != nil {
		if idempotencyKey != "" && isDuplicateKeyError(err) {
			if changed != nil {
				*changed = 0
			}
			return r.loadActionIdempotencyResult(ctx, userID, actionType, idempotencyKey)
		}
		return nil, 0, 0, mapVideoError(err)
	}
	result, err := restoreAction(action)
	if err != nil {
		return nil, 0, 0, err
	}
	return result, likeCount, favoriteCount, nil
}

// GetActionCounts 返回视频当前的点赞数和收藏数。
func (r *Repository) GetActionCounts(ctx context.Context, videoID int64) (int, int, error) {
	if err := r.validateRepository(); err != nil {
		return 0, 0, err
	}
	if videoID <= 0 {
		return 0, 0, domaininteraction.ErrInvalidVideoID
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var stat infravideo.VideoStatModel
	if err := r.db.WithContext(ctx).Where("video_id = ?", videoID).Take(&stat).Error; err != nil {
		return 0, 0, mapVideoError(err)
	}
	return stat.LikeCount, stat.FavoriteCount, nil
}

// CreateComment 创建评论并在同一事务中增加视频评论统计。
func (r *Repository) CreateComment(ctx context.Context, comment *domaininteraction.Comment) (*domaininteraction.Comment, int, error) {
	return r.createComment(ctx, comment, nil)
}

// CreateCommentWithDelta 返回评论和实际新增量，唯一约束冲突重放不触发通知。
func (r *Repository) CreateCommentWithDelta(ctx context.Context, comment *domaininteraction.Comment) (*domaininteraction.Comment, int, int, error) {
	delta := 0
	created, count, err := r.createComment(ctx, comment, &delta)
	return created, count, delta, err
}

// createComment 在事务内保存评论和计数，重复请求读取原评论，成功新增后设置变化量。
func (r *Repository) createComment(ctx context.Context, comment *domaininteraction.Comment, changed *int) (*domaininteraction.Comment, int, error) {
	if err := r.validateRepository(); err != nil {
		return nil, 0, err
	}
	if err := validateComment(comment); err != nil {
		return nil, 0, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var model CommentModel
	var count int
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPublishedVideo(tx, comment.VideoID); err != nil {
			return err
		}

		model = CommentModel{
			VideoID:        comment.VideoID,
			UserID:         comment.UserID,
			Content:        strings.TrimSpace(comment.Content),
			Status:         domaininteraction.CommentStatusNormal,
			IdempotencyKey: idempotencyKeyPtr(comment.IdempotencyKey),
		}
		if err := tx.Create(&model).Error; err != nil {
			return err
		}

		nextCount, statErr := updateVideoStatCounter(tx, model.VideoID, "comment_count", 1)
		count = nextCount
		return statErr
	})
	if err != nil {
		if comment.IdempotencyKey != "" && isDuplicateKeyError(err) {
			existing, existingCount, loadErr := r.FindCommentByUserAndIdempotencyKey(ctx, comment.UserID, comment.IdempotencyKey)
			if loadErr == nil {
				return existing, existingCount, nil
			}
		}
		return nil, 0, mapVideoError(err)
	}

	created, err := r.FindCommentByID(ctx, model.ID)
	if err != nil {
		return nil, 0, err
	}
	if changed != nil {
		*changed = 1
	}
	return created, count, nil
}

// FindCommentByUserAndIdempotencyKey 按用户和幂等键查询已创建评论及当前评论数。
func (r *Repository) FindCommentByUserAndIdempotencyKey(ctx context.Context, userID int64, idempotencyKey string) (*domaininteraction.Comment, int, error) {
	if err := r.validateRepository(); err != nil {
		return nil, 0, err
	}
	if userID <= 0 {
		return nil, 0, domaininteraction.ErrInvalidUserID
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return nil, 0, domaininteraction.ErrCommentNotFound
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var model commentWithUserModel
	result := r.db.WithContext(ctx).
		Table("interaction_comment AS c").
		Select(commentWithUserSelect()).
		Joins("LEFT JOIN users AS u ON u.id = c.user_id").
		Where("c.user_id = ? AND c.idempotency_key = ?", userID, idempotencyKey).
		Take(&model)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, 0, domaininteraction.ErrCommentNotFound
		}
		return nil, 0, result.Error
	}

	count, err := r.commentCount(ctx, model.VideoID)
	if err != nil {
		return nil, 0, err
	}
	return restoreComment(model), count, nil
}

// FindCommentByID 查询指定评论，包括已删除评论及其作者展示信息。
func (r *Repository) FindCommentByID(ctx context.Context, commentID int64) (*domaininteraction.Comment, error) {
	if err := r.validateRepository(); err != nil {
		return nil, err
	}
	if commentID <= 0 {
		return nil, domaininteraction.ErrInvalidCommentID
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var model commentWithUserModel
	err := r.db.WithContext(ctx).
		Table("interaction_comment AS c").
		Select(commentWithUserSelect()).
		Joins("LEFT JOIN users AS u ON u.id = c.user_id").
		Where("c.id = ?", commentID).
		Take(&model).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domaininteraction.ErrCommentNotFound
		}
		return nil, err
	}
	return restoreComment(model), nil
}

// ListComments 按创建时间和 ID 倒序分页查询视频的正常评论。
func (r *Repository) ListComments(ctx context.Context, videoID int64, cursor *domaininteraction.CommentCursor, limit int) ([]*domaininteraction.Comment, error) {
	if err := r.validateRepository(); err != nil {
		return nil, err
	}
	if videoID <= 0 {
		return nil, domaininteraction.ErrInvalidVideoID
	}
	if limit <= 0 {
		return nil, domaininteraction.ErrInvalidLimit
	}
	if limit > domaininteraction.MaxLimit+1 {
		limit = domaininteraction.MaxLimit + 1
	}
	if cursor != nil && (cursor.CreatedAt.IsZero() || cursor.CommentID <= 0) {
		return nil, domaininteraction.ErrInvalidCursor
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var models []commentWithUserModel
	query := r.db.WithContext(ctx).
		Table("interaction_comment AS c").
		Select(commentWithUserSelect()).
		Joins("LEFT JOIN users AS u ON u.id = c.user_id").
		Where("c.video_id = ? AND c.status = ?", videoID, domaininteraction.CommentStatusNormal)
	if cursor != nil {
		query = query.Where("(c.created_at < ? OR (c.created_at = ? AND c.id < ?))", cursor.CreatedAt, cursor.CreatedAt, cursor.CommentID)
	}
	if err := query.Order("c.created_at DESC").Order("c.id DESC").Limit(limit).Scan(&models).Error; err != nil {
		return nil, err
	}

	comments := make([]*domaininteraction.Comment, 0, len(models))
	for _, model := range models {
		comments = append(comments, restoreComment(model))
	}
	return comments, nil
}

// DeleteComment 依据评论作者、视频作者或管理员身份软删除评论，并减少评论统计。
func (r *Repository) DeleteComment(ctx context.Context, commentID int64, userID int64, role string) (*domaininteraction.Comment, int, error) {
	if err := r.validateRepository(); err != nil {
		return nil, 0, err
	}
	if commentID <= 0 {
		return nil, 0, domaininteraction.ErrInvalidCommentID
	}
	if userID <= 0 {
		return nil, 0, domaininteraction.ErrInvalidUserID
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// 先读取视频 ID，再按视频、评论顺序加锁，避免与创建评论事务产生反向锁等待。
	var unlockedComment CommentModel
	if err := r.db.WithContext(ctx).Where("id = ?", commentID).Take(&unlockedComment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, 0, domaininteraction.ErrCommentNotFound
		}
		return nil, 0, err
	}

	role = strings.TrimSpace(role)
	var model CommentModel
	var count int
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var video infravideo.VideoModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", unlockedComment.VideoID).Take(&video).Error; err != nil {
			return mapVideoError(err)
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", commentID).Take(&model).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domaininteraction.ErrCommentNotFound
			}
			return err
		}

		if model.UserID != userID && video.AuthorID != userID && role != domainuser.RoleAdmin {
			return domaininteraction.ErrCommentPermissionDenied
		}
		if model.Status == domaininteraction.CommentStatusDeleted {
			nextCount, statErr := currentVideoStatCounter(tx, model.VideoID, "comment_count")
			count = nextCount
			return statErr
		}

		model.Status = domaininteraction.CommentStatusDeleted
		if err := tx.Save(&model).Error; err != nil {
			return err
		}
		nextCount, statErr := updateVideoStatCounter(tx, model.VideoID, "comment_count", -1)
		count = nextCount
		return statErr
	})
	if err != nil {
		return nil, 0, err
	}

	comment, err := r.FindCommentByID(ctx, model.ID)
	if err != nil {
		return nil, 0, err
	}
	return comment, count, nil
}

// commentCount 返回视频当前维护的评论数量。
func (r *Repository) commentCount(ctx context.Context, videoID int64) (int, error) {
	var stat infravideo.VideoStatModel
	err := r.db.WithContext(ctx).Where("video_id = ?", videoID).Take(&stat).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, domaininteraction.ErrVideoNotFound
		}
		return 0, err
	}
	return stat.CommentCount, nil
}

// lockPublishedVideo 锁定已发布视频，确保互动只能作用于有效视频。
func lockPublishedVideo(tx *gorm.DB, videoID int64) error {
	var video infravideo.VideoModel
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND status = ?", videoID, domainvideo.StatusPublished).
		Take(&video).
		Error
	return mapVideoError(err)
}

// updateActionStat 按动作类型更新点赞或收藏统计。
func updateActionStat(tx *gorm.DB, videoID int64, actionType string, delta int) (int, error) {
	if actionType == domaininteraction.ActionTypeLike {
		return updateVideoStatCounter(tx, videoID, "like_count", delta)
	}
	if actionType == domaininteraction.ActionTypeFavorite {
		return updateVideoStatCounter(tx, videoID, "favorite_count", delta)
	}
	return 0, domaininteraction.ErrInvalidActionType
}

// loadActionIdempotencyResult 读取并恢复已提交的互动动作幂等结果。
func (r *Repository) loadActionIdempotencyResult(ctx context.Context, userID int64, actionType string, idempotencyKey string) (*domaininteraction.Action, int, int, error) {
	var idempotency ActionIdempotencyModel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND action_type = ? AND idempotency_key = ?", userID, actionType, idempotencyKey).
		Take(&idempotency).Error
	if err != nil {
		return nil, 0, 0, err
	}
	return &domaininteraction.Action{
		UserID:         userID,
		VideoID:        idempotency.VideoID,
		ActionType:     actionType,
		Status:         actionStatusFromActive(idempotency.Active),
		IdempotencyKey: idempotencyKey,
	}, idempotency.LikeCount, idempotency.FavoriteCount, nil
}

// updateVideoStatCounter 在锁定统计记录后更新指定计数，并防止计数低于零。
func updateVideoStatCounter(tx *gorm.DB, videoID int64, field string, delta int) (int, error) {
	var stat infravideo.VideoStatModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("video_id = ?", videoID).Take(&stat).Error; err != nil {
		return 0, mapVideoError(err)
	}

	switch field {
	case "like_count":
		stat.LikeCount = clampCount(stat.LikeCount + delta)
	case "favorite_count":
		stat.FavoriteCount = clampCount(stat.FavoriteCount + delta)
	case "comment_count":
		stat.CommentCount = clampCount(stat.CommentCount + delta)
	default:
		return 0, domaininteraction.ErrInvalidActionType
	}

	if err := tx.Save(&stat).Error; err != nil {
		return 0, err
	}
	return statCounter(stat, field), nil
}

// currentVideoStatCounter 读取指定视频统计记录中的一个计数。
func currentVideoStatCounter(tx *gorm.DB, videoID int64, field string) (int, error) {
	var stat infravideo.VideoStatModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("video_id = ?", videoID).Take(&stat).Error; err != nil {
		return 0, mapVideoError(err)
	}
	return statCounter(stat, field), nil
}

// statCounter 返回统计模型中指定字段的值。
func statCounter(stat infravideo.VideoStatModel, field string) int {
	switch field {
	case "like_count":
		return stat.LikeCount
	case "favorite_count":
		return stat.FavoriteCount
	case "comment_count":
		return stat.CommentCount
	default:
		return 0
	}
}

// clampCount 将统计数量限制为不小于零。
func clampCount(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

// actionStatusFromActive 将互动是否生效转换为对应的状态枚举值。
func actionStatusFromActive(active bool) int {
	if active {
		return domaininteraction.ActionStatusActive
	}
	return domaininteraction.ActionStatusCanceled
}

// restoreAction 将动作持久化记录转换为领域对象。
func restoreAction(model ActionModel) (*domaininteraction.Action, error) {
	return domaininteraction.RestoreAction(
		model.ID,
		model.UserID,
		model.VideoID,
		model.ActionType,
		model.Status,
		idempotencyKeyValue(model.IdempotencyKey),
		model.CreatedAt,
		model.UpdatedAt,
	)
}

// restoreComment 将评论查询结果转换为领域对象。
func restoreComment(model commentWithUserModel) *domaininteraction.Comment {
	return domaininteraction.RestoreComment(
		model.ID,
		model.VideoID,
		model.UserID,
		model.UserNickname,
		model.UserAvatarURL,
		model.Content,
		model.Status,
		idempotencyKeyValue(model.IdempotencyKey),
		model.CreatedAt,
		model.UpdatedAt,
	)
}

// commentWithUserSelect 返回评论及作者展示字段的统一查询列清单。
func commentWithUserSelect() string {
	return "c.id, c.video_id, c.user_id, COALESCE(u.nickname, '') AS user_nickname, COALESCE(u.avatar_url, '') AS user_avatar_url, c.content, c.status, c.idempotency_key, c.created_at, c.updated_at"
}

// validateRepository 校验互动仓储及其数据库实例已初始化。
func (r *Repository) validateRepository() error {
	if r == nil || r.db == nil {
		return errors.New("interaction repository is not initialized")
	}
	return nil
}

// validateComment 校验创建评论所需的标识、内容和幂等键。
func validateComment(comment *domaininteraction.Comment) error {
	if comment == nil {
		return errors.New("comment is nil")
	}
	if comment.VideoID <= 0 {
		return domaininteraction.ErrInvalidVideoID
	}
	if comment.UserID <= 0 {
		return domaininteraction.ErrInvalidUserID
	}
	if strings.TrimSpace(comment.Content) == "" {
		return domaininteraction.ErrEmptyCommentContent
	}
	if utf8.RuneCountInString(strings.TrimSpace(comment.Content)) > domaininteraction.MaxCommentContentLength {
		return domaininteraction.ErrCommentContentTooLong
	}
	if len(strings.TrimSpace(comment.IdempotencyKey)) > domaininteraction.MaxIdempotencyKeyLength {
		return domaininteraction.ErrIdempotencyKeyTooLong
	}
	return nil
}

// idempotencyKeyPtr 将空幂等键转换为 nil，避免占用唯一约束。
func idempotencyKeyPtr(value string) *string {
	value = strings.TrimSpace(value)
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

// mapVideoError 将视频查询的记录不存在错误转换为互动领域错误。
func mapVideoError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domaininteraction.ErrVideoNotFound
	}
	return err
}

// isDuplicateKeyError 判断错误是否为 GORM 转换后的唯一键冲突。
func isDuplicateKeyError(err error) bool {
	return errors.Is(err, gorm.ErrDuplicatedKey)
}

var _ domaininteraction.Repository = (*Repository)(nil)

// GetVideoAuthorID 读取公开视频作者 ID，用于互动消息通知。
func (r *Repository) GetVideoAuthorID(ctx context.Context, videoID int64) (int64, error) {
	var video infravideo.VideoModel
	err := r.db.WithContext(ctx).
		Where("id = ? AND status = ?", videoID, domainvideo.StatusPublished).
		Take(&video).
		Error
	if err != nil {
		return 0, mapVideoError(err)
	}
	return video.AuthorID, nil
}

var _ domaininteraction.NotificationRepository = (*Repository)(nil)
