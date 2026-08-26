package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/inumoroha/video-feed/internal/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// New 根据数据库配置创建并连通 PostgreSQL 的 GORM 数据库实例，同时配置底层连接池。
func New(ctx context.Context, cfg config.DatabaseConfig) (*gorm.DB, error) {
	driver := strings.ToLower(strings.TrimSpace(cfg.Driver))
	if driver != "postgres" && driver != "postgresql" {
		return nil, fmt.Errorf("unsupported database driver: %q", cfg.Driver)
	}

	url := strings.TrimSpace(cfg.URL)
	if url == "" {
		return nil, fmt.Errorf("database URL is empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	db, err := gorm.Open(postgres.Open(url), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get database connection: %w", err)
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return db, nil
}
