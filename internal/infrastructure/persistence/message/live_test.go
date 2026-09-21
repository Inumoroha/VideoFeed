package message

import (
	"context"
	"fmt"
	domainmessage "github.com/inumoroha/video-feed/internal/domain/message"
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

// TestMessagePostgresLive 在独立 schema 验证并发事件与请求键去重、空键、跨用户隔离、同时间分页和已读幂等，结束清理资源。
func TestMessagePostgresLive(t *testing.T) {
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
	name := fmt.Sprintf("message_test_%d_%d", os.Getpid(), time.Now().UnixNano())
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
	if err := db.AutoMigrate(&MessageModel{}); err != nil {
		t.Fatal(err)
	}
	repo := New(db)
	ctx := context.Background()
	for _, tc := range []struct{ event, key string }{{"same-event", ""}, {"", "same-key"}} {
		var wg sync.WaitGroup
		var inserted atomic.Int64
		for i := 0; i < 24; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, created, err := repo.Create(ctx, &domainmessage.Message{UserID: 42, Type: "SYSTEM", Title: "first", Content: "body", EventID: tc.event}, tc.key)
				if err != nil {
					t.Error(err)
				}
				if created {
					inserted.Add(1)
				}
			}()
		}
		wg.Wait()
		if inserted.Load() != 1 {
			t.Fatalf("inserted=%d", inserted.Load())
		}
	}
	for i := 0; i < 2; i++ {
		if _, created, err := repo.Create(ctx, &domainmessage.Message{UserID: 42, Type: "SYSTEM", Title: "empty", Content: "body"}, ""); err != nil || !created {
			t.Fatalf("created=%v err=%v", created, err)
		}
	}
	other, created, err := repo.Create(ctx, &domainmessage.Message{UserID: 77, Type: "SYSTEM", Title: "other", Content: "body", EventID: "same-event"}, "same-key")
	if err != nil || !created {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := db.Model(&MessageModel{}).Where("user_id = ?", 42).Update("created_at", now).Error; err != nil {
		t.Fatal(err)
	}
	page, err := repo.ListByUser(ctx, 42, nil, 2)
	if err != nil || len(page) != 2 {
		t.Fatalf("page=%v err=%v", page, err)
	}
	next, err := repo.ListByUser(ctx, 42, &domainmessage.Cursor{CreatedAt: page[1].CreatedAt, MessageID: page[1].ID}, 10)
	if err != nil || len(next) != 2 || next[0].ID >= page[1].ID {
		t.Fatalf("next=%v err=%v", next, err)
	}
	if n, err := repo.MarkRead(ctx, 42, []int64{page[0].ID, other.ID}); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if n, err := repo.MarkRead(ctx, 42, []int64{page[0].ID}); err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if n, err := repo.MarkRead(ctx, 42, nil); err != nil || n != 3 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if n, err := repo.CountUnread(ctx, 77); err != nil || n != 1 {
		t.Fatalf("other unread=%d err=%v", n, err)
	}
}
