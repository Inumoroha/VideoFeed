package exposure

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	domainexposure "github.com/inumoroha/video-feed/internal/domain/exposure"
	feedpersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/feed"
	videopersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/video"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestExposurePostgresLive 在专用连接的隔离 schema 中验证真实并发聚合、重复请求、唯一约束、失败回滚与索引共存，并清理资源。
func TestExposurePostgresLive(t *testing.T) {
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
	name := fmt.Sprintf("exposure_test_%d", time.Now().UnixNano())
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
	if err = db.AutoMigrate(&feedpersistence.InboxModel{}, &videopersistence.VideoModel{}, &ViewEventModel{}, &ExposureModel{}); err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&videopersistence.VideoModel{ID: 1001, AuthorID: 1, Title: "test", MediaURL: "test", CoverURL: "test", Status: 2}).Error; err != nil {
		t.Fatal(err)
	}
	repo := New(db)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			event, err := domainexposure.NewViewEvent(42, 1001, "hot", "repeated", "exposed", 0, false)
			if err != nil {
				t.Error(err)
				return
			}
			if _, _, err = repo.SaveViewEvent(ctx, event); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var aggregate ExposureModel
	if err = db.Where("user_id = ? AND video_id = ?", 42, 1001).Take(&aggregate).Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	if err = db.Model(&ViewEventModel{}).Count(&count).Error; err != nil || count != 32 || aggregate.ExposureCount != 32 {
		t.Fatalf("count=%d aggregate=%+v err=%v", count, aggregate, err)
	}
	first := aggregate.FirstExposedAt
	event, _ := domainexposure.NewViewEvent(42, 1001, "timeline", "repeated", "exposed", 0, false)
	_, again, err := repo.SaveViewEvent(ctx, event)
	if err != nil || again.ExposureCount != 33 || !again.FirstExposedAt.Equal(first) {
		t.Fatalf("again=%+v err=%v", again, err)
	}
	// 故意触发聚合约束失败，确认先插入的流水也被事务回滚。
	if err = db.Exec("ALTER TABLE exposures ADD CONSTRAINT test_count_limit CHECK (exposure_count <= 33)").Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err = repo.SaveViewEvent(ctx, event); err == nil {
		t.Fatal("预期聚合约束失败")
	}
	if err = db.Model(&ViewEventModel{}).Count(&count).Error; err != nil || count != 33 {
		t.Fatalf("回滚后 count=%d err=%v", count, err)
	}
	if err = db.Create(&ExposureModel{UserID: 42, VideoID: 1001, FirstExposedAt: first, LastExposedAt: first, ExposureCount: 1, LastScene: "hot"}).Error; err == nil {
		t.Fatal("唯一约束未生效")
	}
}
