package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/inumoroha/video-feed/internal/config"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// New 根据数据库配置创建 PostgreSQL 连接池，连接检测失败时关闭已创建的资源。
func New(ctx context.Context, cfg config.DatabaseConfig) (*sql.DB, error) {
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

	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxIdleConns(10)
	db.SetMaxOpenConns(50)
	db.SetConnMaxIdleTime(5 * time.Minute)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return db, nil
}
