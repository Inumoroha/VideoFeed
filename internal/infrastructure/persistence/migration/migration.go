package migration

import (
	"fmt"
	embeddingpersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/embedding"
	exposurepersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/exposure"
	feedpersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/feed"
	interactionpersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/interaction"
	messagepersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/message"
	relationpersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/relation"
	userpersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/user"
	videopersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/video"
	"gorm.io/gorm"
)

// AutoMigrate 在事务内串行迁移全部业务表并补齐统计；事务级咨询锁避免 API 与 worker 并发启动竞争，失败自动回滚并释放锁。
func AutoMigrate(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("gorm database is nil")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(736496102)").Error; err != nil {
			return fmt.Errorf("lock migrations: %w", err)
		}
		return migrateTables(tx)
	})
}

// migrateTables 保留目标仓库模型、幂等表和错误上下文，模型迁移同时创建时间线索引，最后补齐视频统计。
func migrateTables(gormDB *gorm.DB) error {
	if err := gormDB.AutoMigrate(&userpersistence.UserModel{}); err != nil {
		return fmt.Errorf("migrate users: %w", err)
	}
	if err := gormDB.AutoMigrate(&videopersistence.VideoModel{}); err != nil {
		return fmt.Errorf("migrate videos: %w", err)
	}
	if err := gormDB.AutoMigrate(&videopersistence.VideoStatModel{}); err != nil {
		return fmt.Errorf("migrate video stats: %w", err)
	}
	if err := gormDB.AutoMigrate(&interactionpersistence.ActionModel{}, &interactionpersistence.ActionIdempotencyModel{}, &interactionpersistence.CommentModel{}); err != nil {
		return fmt.Errorf("migrate interactions: %w", err)
	}
	if err := gormDB.AutoMigrate(&relationpersistence.FollowModel{}, &relationpersistence.RelationStatModel{}, &relationpersistence.FollowIdempotencyModel{}); err != nil {
		return fmt.Errorf("migrate relations: %w", err)
	}
	if err := gormDB.AutoMigrate(&messagepersistence.MessageModel{}); err != nil {
		return fmt.Errorf("migrate messages: %w", err)
	}
	if err := gormDB.AutoMigrate(&feedpersistence.InboxModel{}); err != nil {
		return fmt.Errorf("migrate feed inbox: %w", err)
	}
	if err := gormDB.AutoMigrate(&exposurepersistence.ViewEventModel{}, &exposurepersistence.ExposureModel{}); err != nil {
		return fmt.Errorf("migrate exposures: %w", err)
	}
	if err := gormDB.AutoMigrate(&embeddingpersistence.VideoEmbeddingModel{}); err != nil {
		return fmt.Errorf("migrate video embeddings: %w", err)
	}
	if err := videopersistence.EnsureStats(gormDB); err != nil {
		return fmt.Errorf("ensure video stats: %w", err)
	}

	return nil
}
