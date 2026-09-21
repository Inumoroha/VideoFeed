package playback

import (
	"context"
	"fmt"
	domain "github.com/inumoroha/video-feed/internal/domain/playback"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestPlaybackPostgresLive 在独立数据库模式中验证唯一索引、并发重放、空键及跨用户隔离，结束释放连接并清理模式。
func TestPlaybackPostgresLive(t *testing.T) {
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
	name := fmt.Sprintf("playback_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec("CREATE SCHEMA " + quoted); err != nil {
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
	// 预置旧索引名，确认新模块的唯一约束不受已有模块影响。
	if err = db.Exec("CREATE TABLE existing_keys (id BIGINT); CREATE UNIQUE INDEX uk_user_idempotency ON existing_keys (id)").Error; err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&ConfigModel{}, &QoSLogModel{}); err != nil {
		t.Fatal(err)
	}
	repo := New(db)
	ctx := context.Background()
	var wg sync.WaitGroup
	var inserted atomic.Int64
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, created, err := repo.CreateQoSReport(ctx, &domain.QoSReport{UserID: 42, VideoID: 1, IdempotencyKey: "same", WatchMs: 10})
			if err != nil {
				t.Error(err)
				return
			}
			if got.WatchMs != 10 {
				t.Error(got)
			}
			if created {
				inserted.Add(1)
			}
		}()
	}
	wg.Wait()
	if inserted.Load() != 1 {
		t.Fatalf("并发新建次数 %d", inserted.Load())
	}
	got, created, err := repo.CreateQoSReport(ctx, &domain.QoSReport{UserID: 42, VideoID: 2, IdempotencyKey: "same", WatchMs: 999})
	if err != nil || created || got.VideoID != 1 || got.WatchMs != 10 {
		t.Fatalf("%+v %v %v", got, created, err)
	}
	for i := 0; i < 2; i++ {
		if _, created, err := repo.CreateQoSReport(ctx, &domain.QoSReport{UserID: 42, VideoID: 1}); err != nil || !created {
			t.Fatalf("空键 %v %v", created, err)
		}
	}
	if _, created, err := repo.CreateQoSReport(ctx, &domain.QoSReport{UserID: 77, VideoID: 1, IdempotencyKey: "same"}); err != nil || !created {
		t.Fatalf("跨用户 %v %v", created, err)
	}
	if err = db.Create(&ConfigModel{Platform: "Web", NetworkType: "DEFAULT", PreloadCount: 3, BufferMs: 1200}).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&ConfigModel{Platform: "Web", NetworkType: "DEFAULT", PreloadCount: 5}).Error; err == nil {
		t.Fatal("配置唯一约束缺失")
	}
}
