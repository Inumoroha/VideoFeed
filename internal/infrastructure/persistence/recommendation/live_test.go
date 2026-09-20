package recommendation

import (
	"context"
	"errors"
	"fmt"
	domain "github.com/inumoroha/video-feed/internal/domain/recommendation"
	embedding "github.com/inumoroha/video-feed/internal/infrastructure/persistence/embedding"
	exposure "github.com/inumoroha/video-feed/internal/infrastructure/persistence/exposure"
	video "github.com/inumoroha/video-feed/internal/infrastructure/persistence/video"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
	"sync"
	"testing"
	"time"
)

// TestRecommendationPostgresLive 在独立 schema 验证并发曝光计数、唯一约束、批量回滚、失败重试和七日过滤、曝光决策边界与并发只读行为，并关闭连接清理数据。
func TestRecommendationPostgresLive(t *testing.T) {
	url := os.Getenv("VIDEOFEED_TEST_POSTGRES_URL")
	if url == "" {
		t.Skip("未配置专用 PostgreSQL 测试连接")
	}
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatal("解析测试数据库连接失败")
	}
	admin := stdlib.OpenDB(*cfg)
	defer admin.Close()
	name := fmt.Sprintf("recommendation_test_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec("CREATE SCHEMA " + quoted); err != nil {
		t.Fatal("创建隔离 schema 失败")
	}
	defer func() {
		if _, err := admin.Exec("DROP SCHEMA " + quoted + " CASCADE"); err != nil {
			t.Error("清理测试 schema 失败")
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
	if err = db.AutoMigrate(&video.VideoModel{}, &video.VideoStatModel{}, &embedding.VideoEmbeddingModel{}, &exposure.ViewEventModel{}, &exposure.ExposureModel{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, id := range []int64{1, 2} {
		if err = db.Create(&video.VideoModel{ID: id, AuthorID: 10, Title: "test", MediaURL: "test", CoverURL: "test", Status: 2, PublishedAt: &now}).Error; err != nil {
			t.Fatal(err)
		}
	}
	repo := New(db)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	write := &domain.ExposureWrite{UserID: 42, VideoID: 1, Scene: "recommend", RequestID: "same"}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := repo.SaveExposures(ctx, []*domain.ExposureWrite{write}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var aggregate exposure.ExposureModel
	if err = db.Where("user_id = ? AND video_id = ?", 42, 1).Take(&aggregate).Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	if err = db.Model(&exposure.ViewEventModel{}).Count(&count).Error; err != nil || count != 32 || aggregate.ExposureCount != 32 {
		t.Fatal(count, aggregate, err)
	}
	duplicate := exposure.ExposureModel{UserID: 42, VideoID: 1, FirstExposedAt: now, LastExposedAt: now, ExposureCount: 1, LastScene: "recommend"}
	if err = db.Create(&duplicate).Error; err == nil {
		t.Fatal("唯一键未生效")
	}
	if _, err = repo.SaveExposures(ctx, []*domain.ExposureWrite{{UserID: 42, VideoID: 2, Scene: "recommend"}, {UserID: 42, VideoID: 404, Scene: "recommend"}}); !errors.Is(err, domain.ErrVideoNotFound) {
		t.Fatal(err)
	}
	if err = db.Model(&exposure.ViewEventModel{}).Count(&count).Error; err != nil || count != 32 {
		t.Fatal("批次未回滚", count, err)
	}
	saved, err := repo.SaveExposures(ctx, []*domain.ExposureWrite{write})
	if err != nil || saved[0].ExposureCount != 33 || !saved[0].FirstExposedAt.Equal(aggregate.FirstExposedAt) {
		t.Fatal(saved, err)
	}
	pool, err := repo.ListCandidatePool(ctx, 42, 50)
	if err != nil || len(pool) != 1 || pool[0].VideoID != 2 {
		t.Fatal(pool, err)
	}
	if err = db.Model(&exposure.ExposureModel{}).Where("user_id = ?", 42).Update("last_exposed_at", now.Add(-8*24*time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	pool, err = repo.ListCandidatePool(ctx, 42, 50)
	if err != nil || len(pool) != 2 {
		t.Fatal(pool, err)
	}
	verifyExposureDecisionsLive(t, db)
}
