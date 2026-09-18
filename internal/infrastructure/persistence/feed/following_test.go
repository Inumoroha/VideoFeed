package feed

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
)

const followingSQL = `SELECT merged.video_id, merged.published_at FROM (
SELECT v.id AS video_id, v.published_at AS published_at
FROM feed_inbox AS fi JOIN videos AS v ON v.id = fi.video_id
JOIN user_follow AS f ON f.user_id = fi.user_id AND f.target_user_id = v.author_id
WHERE fi.user_id = $1 AND f.status = $2 AND v.status = $3 AND v.published_at IS NOT NULL
UNION
SELECT v.id AS video_id, v.published_at AS published_at
FROM videos AS v JOIN user_follow AS f ON f.target_user_id = v.author_id
JOIN user_relation_stat AS rs ON rs.user_id = v.author_id
WHERE f.user_id = $4 AND f.status = $5 AND rs.follower_count >= $6 AND v.status = $7 AND v.published_at IS NOT NULL
) AS merged`

// TestListFollowingPageSQL 验证真实表名、推拉去重、状态过滤、阈值、游标排序和错误传播。
func TestListFollowingPageSQL(t *testing.T) {
	r, mock := mockRepo(t)
	at := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(followingSQL+` ORDER BY merged.published_at DESC, merged.video_id DESC LIMIT $8`)).
		WithArgs(42, 1, 2, 42, 1, 10000, 2, 101).
		WillReturnRows(sqlmock.NewRows([]string{"video_id", "published_at"}).AddRow(7, at))
	items, err := r.ListFollowingPage(nil, 42, nil, 999)
	if err != nil || len(items) != 1 || items[0].VideoID != 7 || !items[0].PublishedAt.Equal(at) {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	query := followingSQL + ` WHERE (merged.published_at < $8 OR (merged.published_at = $9 AND merged.video_id < $10)) ORDER BY merged.published_at DESC, merged.video_id DESC LIMIT $11`
	cursor := &domainfeed.TimelineCursor{PublishedAt: at, VideoID: 7}
	failure := errors.New("database unavailable")
	mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(42, 1, 2, 42, 1, 10000, 2, at, at, 7, 2).WillReturnError(failure)
	if _, err := r.ListFollowingPage(context.Background(), 42, cursor, 2); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(42, 1, 2, 42, 1, 10000, 2, at, at, 7, 2).WillReturnRows(sqlmock.NewRows([]string{"video_id", "published_at"}).AddRow(6, at))
	items, err = r.ListFollowingPage(context.Background(), 42, cursor, 2)
	if err != nil || len(items) != 1 || items[0].VideoID != 6 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestFollowingRepositoryValidation 验证非法仓储、用户、分页和游标在访问数据库前被拒绝。
func TestFollowingRepositoryValidation(t *testing.T) {
	var uninitialized *Repository
	if _, err := uninitialized.ListFollowingPage(nil, 42, nil, 1); err == nil {
		t.Fatal("expected initialization error")
	}
	r, mock := mockRepo(t)
	if _, err := r.ListFollowingPage(nil, 0, nil, 1); !errors.Is(err, domainfeed.ErrViewerRequired) {
		t.Fatal(err)
	}
	if _, err := r.ListFollowingPage(nil, 42, nil, 0); !errors.Is(err, domainfeed.ErrInvalidLimit) {
		t.Fatal(err)
	}
	if _, err := r.ListFollowingPage(nil, 42, &domainfeed.TimelineCursor{}, 1); !errors.Is(err, domainfeed.ErrInvalidCursor) {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestInboxUniqueIndex 验证 GORM 为用户与视频建立联合唯一索引，供原子冲突更新使用。
func TestInboxUniqueIndex(t *testing.T) {
	r, mock := mockRepo(t)
	if (InboxModel{}).TableName() != "feed_inbox" {
		t.Fatal("unexpected table name")
	}
	mock.ExpectExec(regexp.QuoteMeta(`CREATE UNIQUE INDEX IF NOT EXISTS "uk_user_video" ON "feed_inbox" ("user_id","video_id")`)).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := r.db.Migrator().CreateIndex(&InboxModel{}, "uk_user_video"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
