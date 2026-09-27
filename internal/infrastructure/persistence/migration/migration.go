package migration

import (
	"errors"
	"fmt"
	embeddingpersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/embedding"
	exposurepersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/exposure"
	feedpersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/feed"
	interactionpersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/interaction"
	messagepersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/message"
	playbackpersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/playback"
	relationpersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/relation"
	userpersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/user"
	videopersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/video"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"time"
)

const migrationRetryAttempts = 4

// pgDuplicateColumn 表示 PostgreSQL 并发建表时列已存在的错误码。
const pgDuplicateColumn = "42701"

// pgDuplicateTable 表示 PostgreSQL 并发建表或建索引时对象已存在的错误码。
const pgDuplicateTable = "42P07"

// pgUniqueViolation 表示并发创建唯一索引时唯一约束冲突的错误码。
const pgUniqueViolation = "23505"

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
// 并发建表或建索引产生的 PostgreSQL 冲突错误会退避重试，其余错误立即返回。
func migrateTables(gormDB *gorm.DB) error {
	if err := migrateTablesWithRetry(gormDB, migrateModels); err != nil {
		return err
	}
	if err := videopersistence.EnsureStats(gormDB); err != nil {
		return fmt.Errorf("ensure video stats: %w", err)
	}

	return nil
}

// migrateTablesWithRetry 按次数尝试迁移动作，仅在并发 DDL 冲突时退避重试并保留最后一次错误。
func migrateTablesWithRetry(gormDB *gorm.DB, migrate func(*gorm.DB) error) error {
	var err error
	for attempt := 0; attempt < migrationRetryAttempts; attempt++ {
		err = migrate(gormDB)
		if err == nil {
			return nil
		}
		if !isConcurrentMigrationError(err) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 150 * time.Millisecond)
	}
	return err
}

// isConcurrentMigrationError 判断错误是否为 PostgreSQL 并发 DDL 产生的列、对象或唯一约束冲突。
func isConcurrentMigrationError(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	switch pgErr.Code {
	case pgDuplicateColumn, pgDuplicateTable, pgUniqueViolation:
		return true
	default:
		return false
	}
}

// migrateModels 按目标仓库表顺序迁移全部业务模型，返回首个失败表的错误上下文。
func migrateModels(gormDB *gorm.DB) error {
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
	if err := gormDB.AutoMigrate(&playbackpersistence.ConfigModel{}, &playbackpersistence.QoSLogModel{}); err != nil {
		return fmt.Errorf("migrate playback: %w", err)
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

	return nil
}
