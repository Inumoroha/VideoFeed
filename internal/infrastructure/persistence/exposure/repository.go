package exposure

import (
	"context"
	"errors"
	domainexposure "github.com/inumoroha/video-feed/internal/domain/exposure"
	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"
	videopersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/video"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository 通过 GORM 事务保存观看行为及曝光聚合。
type Repository struct {
	db *gorm.DB
}

// New 创建使用指定数据库的曝光仓储。
func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// SaveViewEvent 在同一事务中校验视频、追加流水并原子累加曝光；任一步失败均回滚，重复请求不去重。
func (r *Repository) SaveViewEvent(ctx context.Context, event *domainexposure.ViewEvent) (*domainexposure.ViewEvent, *domainexposure.Exposure, error) {
	var eventModel ViewEventModel
	var exposureModel ExposureModel
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensurePublishedVideo(tx, event.VideoID); err != nil {
			return err
		}

		eventModel = ViewEventModel{
			UserID:    event.UserID,
			VideoID:   event.VideoID,
			Scene:     event.Scene,
			RequestID: stringPtr(event.RequestID),
			EventType: event.EventType,
			WatchMs:   event.WatchMs,
			Completed: event.Completed,
		}
		if err := tx.Create(&eventModel).Error; err != nil {
			return err
		}

		if !event.CountsAsExposure() {
			return nil
		}

		exposureModel = ExposureModel{
			UserID:         event.UserID,
			VideoID:        event.VideoID,
			FirstExposedAt: eventModel.CreatedAt,
			LastExposedAt:  eventModel.CreatedAt,
			ExposureCount:  1,
			LastScene:      event.Scene,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "user_id"},
				{Name: "video_id"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				"last_exposed_at": gorm.Expr("EXCLUDED.last_exposed_at"),
				"exposure_count":  gorm.Expr("exposures.exposure_count + 1"),
				"last_scene":      gorm.Expr("EXCLUDED.last_scene"),
				"updated_at":      gorm.Expr("EXCLUDED.updated_at"),
			}),
		}).Create(&exposureModel).Error; err != nil {
			return err
		}

		return tx.Where("user_id = ? AND video_id = ?", event.UserID, event.VideoID).Take(&exposureModel).Error
	})
	if err != nil {
		return nil, nil, mapExposureError(err)
	}

	savedEvent := restoreViewEvent(eventModel)
	var exposure *domainexposure.Exposure
	if event.CountsAsExposure() {
		exposure = restoreExposure(exposureModel)
	}
	return savedEvent, exposure, nil
}

// ensurePublishedVideo 检查视频是否处于已发布状态，不存在或不可见时返回领域错误。
func ensurePublishedVideo(tx *gorm.DB, videoID int64) error {
	var video videopersistence.VideoModel
	err := tx.Select("id").
		Where("id = ? AND status = ?", videoID, domainvideo.StatusPublished).
		Take(&video).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domainexposure.ErrVideoNotFound
		}
		return err
	}
	return nil
}

// restoreViewEvent 将数据库观看流水转换为领域对象。
func restoreViewEvent(model ViewEventModel) *domainexposure.ViewEvent {
	return domainexposure.RestoreViewEvent(
		model.ID,
		model.UserID,
		model.VideoID,
		model.Scene,
		stringValue(model.RequestID),
		model.EventType,
		model.WatchMs,
		model.Completed,
		model.CreatedAt,
	)
}

// restoreExposure 将数据库曝光聚合转换为领域对象。
func restoreExposure(model ExposureModel) *domainexposure.Exposure {
	return domainexposure.RestoreExposure(
		model.ID,
		model.UserID,
		model.VideoID,
		model.FirstExposedAt,
		model.LastExposedAt,
		model.ExposureCount,
		model.LastScene,
		model.CreatedAt,
		model.UpdatedAt,
	)
}

// stringPtr 将空字符串转换为空指针，其余返回字符串指针。
func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// stringValue 读取可空字符串，空指针返回空字符串。
func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// mapExposureError 保留曝光领域错误及数据库原始错误。
func mapExposureError(err error) error {
	if errors.Is(err, domainexposure.ErrVideoNotFound) {
		return err
	}
	if isDuplicateKeyError(err) {
		return err
	}
	return err
}

// isDuplicateKeyError 识别 GORM 或 PostgreSQL 唯一键冲突。
func isDuplicateKeyError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
