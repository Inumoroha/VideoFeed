package message

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	domainmessage "github.com/inumoroha/video-feed/internal/domain/message"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"testing"
	"time"
)

// newMessageSQLMock 创建使用真实 PostgreSQL 方言的模拟数据库，并在测试结束检查所有预期。
func newMessageSQLMock(t *testing.T) (*Repository, sqlmock.Sqlmock) {
	t.Helper()
	db, m, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	orm, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := m.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		db.Close()
	})
	return New(orm), m
}

// TestCreateDuplicateRollback 验证唯一冲突先回滚，再按当前用户及事件或请求键查询既有消息。
func TestCreateDuplicateRollback(t *testing.T) {
	r, m := newMessageSQLMock(t)
	ctx := context.Background()
	now := time.Now().UTC()
	m.ExpectBegin()
	m.ExpectQuery(`INSERT INTO "user_message"`).WillReturnError(&pgconn.PgError{Code: "23505"})
	m.ExpectRollback()
	m.ExpectQuery(`SELECT .* FROM "user_message" WHERE user_id = \$1 AND \(event_id = \$2 OR idempotency_key = \$3\) ORDER BY id DESC LIMIT \$4 FOR UPDATE`).WithArgs(int64(42), "evt", "key", 1).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "type", "title", "content", "event_id", "created_at"}).AddRow(1, 42, "LIKE", "first", "body", "evt", now))
	item, created, err := r.Create(ctx, &domainmessage.Message{UserID: 42, Type: "LIKE", Title: "changed", Content: "body", EventID: "evt"}, "key")
	if err != nil || created || item.ID != 1 || item.Title != "first" {
		t.Fatalf("item=%+v created=%v err=%v", item, created, err)
	}
}

// TestCreateFailureRetry 验证普通数据库失败回滚且不执行重复查询，后续调用可重新成功写入。
func TestCreateFailureRetry(t *testing.T) {
	r, m := newMessageSQLMock(t)
	failure := errors.New("database unavailable")
	msg := &domainmessage.Message{UserID: 42, Type: "SYSTEM", Title: "t", Content: "c"}
	m.ExpectBegin()
	m.ExpectQuery(`INSERT INTO "user_message"`).WillReturnError(failure)
	m.ExpectRollback()
	if _, _, err := r.Create(context.Background(), msg, ""); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	m.ExpectBegin()
	m.ExpectQuery(`INSERT INTO "user_message"`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
	m.ExpectCommit()
	item, created, err := r.Create(context.Background(), msg, "")
	if err != nil || !created || item.ID != 2 {
		t.Fatalf("item=%+v created=%v err=%v", item, created, err)
	}
}

// TestMarkReadIsolation 验证更新限定当前用户和未读状态，并传播事务错误。
func TestMarkReadIsolation(t *testing.T) {
	r, m := newMessageSQLMock(t)
	m.ExpectBegin()
	m.ExpectExec(`UPDATE "user_message" SET .* WHERE \(user_id = \$3 AND is_read = \$4\) AND id IN \(\$5,\$6\)`).WithArgs(true, sqlmock.AnyArg(), int64(42), false, int64(1), int64(2)).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectCommit()
	n, err := r.MarkRead(context.Background(), 42, []int64{1, 2})
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	m.ExpectBegin()
	m.ExpectExec(`UPDATE "user_message"`).WillReturnError(errors.New("update failed"))
	m.ExpectRollback()
	if _, err := r.MarkRead(context.Background(), 42, nil); err == nil {
		t.Fatal("missing failure")
	}
}

// TestMessageRepositoryReadErrors 验证列表和计数传播读取错误。
func TestMessageRepositoryReadErrors(t *testing.T) {
	r, m := newMessageSQLMock(t)
	failure := errors.New("read failed")
	m.ExpectQuery(`SELECT .* FROM "user_message"`).WillReturnError(failure)
	if _, err := r.ListByUser(context.Background(), 42, &domainmessage.Cursor{CreatedAt: time.Now(), MessageID: 2}, 20); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	m.ExpectQuery(`SELECT count\(\*\) FROM "user_message"`).WillReturnError(failure)
	if _, err := r.CountUnread(context.Background(), 42); !errors.Is(err, failure) {
		t.Fatal(err)
	}
}
