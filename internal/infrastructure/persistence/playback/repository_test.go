package playback

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	domain "github.com/inumoroha/video-feed/internal/domain/playback"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"testing"
	"time"
)

// newSQLMock 创建 PostgreSQL 模拟连接，并在结束时验证预期和关闭连接。
func newSQLMock(t *testing.T) (*Repository, sqlmock.Sqlmock) {
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

// TestDuplicateAndFailureRetry 验证插入回滚后按用户和键读取首次结果，普通失败可由调用方重试。
func TestDuplicateAndFailureRetry(t *testing.T) {
	r, m := newSQLMock(t)
	ctx := context.Background()
	report := &domain.QoSReport{UserID: 42, VideoID: 7, IdempotencyKey: "same", WatchMs: 999}
	m.ExpectBegin()
	m.ExpectQuery(`INSERT INTO "playback_qos_log"`).WillReturnError(&pgconn.PgError{Code: "23505"})
	m.ExpectRollback()
	m.ExpectQuery(`SELECT .* FROM "playback_qos_log" WHERE user_id = \$1 AND idempotency_key = \$2 ORDER BY id DESC LIMIT \$3 FOR UPDATE`).WithArgs(int64(42), "same", 1).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "video_id", "watch_ms", "idempotency_key"}).AddRow(1, 42, 7, 12, "same"))
	got, created, err := r.CreateQoSReport(ctx, report)
	if err != nil || created || got.WatchMs != 12 || got.ID != 1 {
		t.Fatalf("%+v %v %v", got, created, err)
	}
	failure := errors.New("unavailable")
	m.ExpectBegin()
	m.ExpectQuery(`INSERT INTO "playback_qos_log"`).WillReturnError(failure)
	m.ExpectRollback()
	if _, _, err = r.CreateQoSReport(ctx, report); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	m.ExpectBegin()
	m.ExpectQuery(`INSERT INTO "playback_qos_log"`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
	m.ExpectCommit()
	if got, created, err = r.CreateQoSReport(ctx, report); err != nil || !created || got.ID != 2 {
		t.Fatalf("%+v %v %v", got, created, err)
	}
}

// TestConfigAndPreloadQueries 验证配置缺失、实际视频表名、双字段游标和无游标回退。
func TestConfigAndPreloadQueries(t *testing.T) {
	r, m := newSQLMock(t)
	ctx := context.Background()
	now := time.Now()
	m.ExpectQuery(`SELECT .* FROM "playback_config"`).WithArgs("Web", "WiFi", 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if got, err := r.FindConfig(ctx, "Web", "WiFi"); err != nil || got != nil {
		t.Fatalf("%v %v", got, err)
	}
	m.ExpectQuery(`SELECT .* FROM videos AS v WHERE .*`).WithArgs(int64(9), 2, 1).WillReturnRows(sqlmock.NewRows([]string{"video_id", "published_at"}).AddRow(9, now))
	m.ExpectQuery(`SELECT .* FROM videos AS v WHERE .*v.published_at < .*v.published_at = .*v.id < .* ORDER BY v.published_at DESC,v.id DESC LIMIT \$5`).WithArgs(2, now, now, int64(9), 3).WillReturnRows(sqlmock.NewRows([]string{"video_id", "media_url", "cover_url"}).AddRow(8, " m ", " c "))
	got, err := r.ListPreloadVideos(ctx, 9, 3)
	if err != nil || len(got) != 1 || got[0].MediaURL != "m" {
		t.Fatalf("%+v %v", got, err)
	}
	m.ExpectQuery(`SELECT .* FROM videos AS v WHERE .*`).WithArgs(int64(404), 2, 1).WillReturnRows(sqlmock.NewRows([]string{"video_id", "published_at"}))
	m.ExpectQuery(`SELECT .* FROM videos AS v WHERE v.status = \$1 AND v.published_at IS NOT NULL ORDER BY v.published_at DESC,v.id DESC LIMIT \$2`).WithArgs(2, 3).WillReturnRows(sqlmock.NewRows([]string{"video_id"}))
	got, err = r.ListPreloadVideos(ctx, 404, 3)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("%+v %v", got, err)
	}
}

// TestNullableKeysAndDuplicateErrors 验证空键转换与 PostgreSQL、GORM 和旧错误文本的兼容识别。
func TestNullableKeysAndDuplicateErrors(t *testing.T) {
	if optionalString(" ") != nil || stringValue(nil) != "" || stringValue(optionalString(" k ")) != "k" {
		t.Fatal("空键语义改变")
	}
	for _, err := range []error{gorm.ErrDuplicatedKey, &pgconn.PgError{Code: "23505"}, errors.New("duplicate key")} {
		if !isDuplicateKeyError(err) {
			t.Fatal(err)
		}
	}
	if isDuplicateKeyError(errors.New("offline")) {
		t.Fatal("误识别普通错误")
	}
}
