package embedding

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	domainembedding "github.com/inumoroha/video-feed/internal/domain/embedding"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestEmbeddingPostgresLive 在独立 schema 验证真实并发覆盖、模型隔离、创建时间保留、JSON 约束及错误恢复，结束后清理。
func TestEmbeddingPostgresLive(t *testing.T) {
	url := os.Getenv("VIDEOFEED_TEST_POSTGRES_URL")
	if url == "" {
		t.Skip("未配置专用 PostgreSQL 测试连接")
	}
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatal("解析测试连接失败")
	}
	admin := stdlib.OpenDB(*cfg)
	defer admin.Close()
	name := fmt.Sprintf("embedding_test_%d", os.Getpid())
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec("CREATE SCHEMA " + quoted); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec("DROP SCHEMA " + quoted + " CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	cfg.RuntimeParams["search_path"] = name
	sqlDB := stdlib.OpenDB(*cfg)
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(8)
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&VideoEmbeddingModel{}); err != nil {
		t.Fatal(err)
	}
	repo := New(db)
	ctx := context.Background()
	if _, err := repo.FindVideoEmbedding(ctx, 1, "model-a"); !errors.Is(err, domainembedding.ErrVideoEmbeddingNotFound) {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value := domainembedding.NewVideoEmbedding(1, "model-a", []float64{1}, "first", "[1]")
			if err := repo.SaveVideoEmbedding(ctx, value); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var count int64
	if err = db.Model(&VideoEmbeddingModel{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	first, err := repo.FindVideoEmbedding(ctx, 1, "model-a")
	if err != nil {
		t.Fatal(err)
	}
	next := domainembedding.NewVideoEmbedding(1, "model-a", []float64{0, 1}, "second", "[0,1]")
	if err = repo.SaveVideoEmbedding(ctx, next); err != nil {
		t.Fatal(err)
	}
	updated, err := repo.FindVideoEmbedding(ctx, 1, "model-a")
	if err != nil || updated.Dimension != 2 || updated.EmbeddingJSON != "[0,1]" || updated.Embedding != nil || updated.TextHash != "second" || !updated.CreatedAt.Equal(first.CreatedAt) || updated.UpdatedAt.Before(first.UpdatedAt) {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	next.Model = "model-b"
	if err = repo.SaveVideoEmbedding(ctx, next); err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&VideoEmbeddingModel{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	next.Model = "model-a"
	next.EmbeddingJSON = "not-json"
	next.TextHash = "bad"
	if err = repo.SaveVideoEmbedding(ctx, next); err == nil {
		t.Fatal("非法 JSON 未被拒绝")
	}
	restored, err := repo.FindVideoEmbedding(ctx, 1, "model-a")
	if err != nil || restored.TextHash != "second" {
		t.Fatal(restored, err)
	}
	next.EmbeddingJSON = "[0,1]"
	next.TextHash = "recovered"
	if err = repo.SaveVideoEmbedding(ctx, next); err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&VideoEmbeddingModel{VideoID: 1, Model: "model-a", Dimension: 1, EmbeddingJSON: "[1]", TextHash: "duplicate"}).Error; err == nil {
		t.Fatal("复合唯一约束未生效")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err = repo.SaveVideoEmbedding(cancelled, next); err == nil {
		t.Fatal("取消上下文仍写入成功")
	}
}
