package feed

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestRepositoryRejectsUninitializedRepository 验证未初始化仓储不会触发空指针异常。
func TestRepositoryRejectsUninitializedRepository(t *testing.T) {
	var repository *Repository

	if _, err := repository.ListTimeFeed(context.TODO(), nil, 10); err == nil || err.Error() != "feed repository is not initialized" {
		t.Fatalf("ListTimeFeed() error = %v, want uninitialized repository error", err)
	}
	if err := repository.SaveViewEvent(context.TODO(), &domainfeed.ViewEvent{}); err == nil || err.Error() != "feed repository is not initialized" {
		t.Fatalf("SaveViewEvent() error = %v, want uninitialized repository error", err)
	}
	if _, err := repository.FindViewEventByIdempotencyKey(context.TODO(), "key"); err == nil || err.Error() != "feed repository is not initialized" {
		t.Fatalf("FindViewEventByIdempotencyKey() error = %v, want uninitialized repository error", err)
	}
}

// TestRepositoryRejectsInvalidInput 验证仓储会拒绝非法分页、游标和空事件参数。
func TestRepositoryRejectsInvalidInput(t *testing.T) {
	repository, _ := newMockRepository(t)

	if _, err := repository.ListTimeFeed(context.TODO(), nil, 0); !errors.Is(err, domainfeed.ErrInvalidLimit) {
		t.Fatalf("ListTimeFeed() error = %v, want %v", err, domainfeed.ErrInvalidLimit)
	}
	if _, err := repository.ListTimeFeed(context.TODO(), &domainfeed.TimeCursor{VideoID: 1}, 10); !errors.Is(err, domainfeed.ErrInvalidCursor) {
		t.Fatalf("ListTimeFeed() error = %v, want %v", err, domainfeed.ErrInvalidCursor)
	}
	if err := repository.SaveViewEvent(context.TODO(), nil); err == nil || err.Error() != "view event is nil" {
		t.Fatalf("SaveViewEvent() error = %v, want nil event error", err)
	}
	if _, err := repository.FindViewEventByIdempotencyKey(context.TODO(), " "); !errors.Is(err, domainfeed.ErrViewEventNotFound) {
		t.Fatalf("FindViewEventByIdempotencyKey() error = %v, want %v", err, domainfeed.ErrViewEventNotFound)
	}
}

// TestListTimeFeedMapsPublishedVideos 验证信息流查询使用正确的视频表并映射返回字段。
func TestListTimeFeedMapsPublishedVideos(t *testing.T) {
	repository, mock := newMockRepository(t)
	publishedAt := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id AS video_id, author_id, title, media_url, cover_url, like_count, comment_count, favorite_count, published_at FROM "videos" WHERE status = $1 AND published_at IS NOT NULL ORDER BY published_at DESC,id DESC LIMIT $2`)).
		WithArgs(domainvideo.StatusPublished, 2).
		WillReturnRows(sqlmock.NewRows([]string{"video_id", "author_id", "title", "media_url", "cover_url", "like_count", "comment_count", "favorite_count", "published_at"}).
			AddRow(7, 42, "title", "media", "cover", 3, 4, 5, publishedAt))

	items, err := repository.ListTimeFeed(context.Background(), nil, 2)
	if err != nil {
		t.Fatalf("ListTimeFeed() error = %v", err)
	}
	if len(items) != 1 || items[0].VideoID != 7 || items[0].AuthorID != 42 || items[0].Title != "title" || !items[0].PublishedAt.Equal(publishedAt) {
		t.Fatalf("ListTimeFeed() items = %+v, want mapped video item", items)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

// TestListTimeFeedCapsLimit 验证超过领域上限的分页数量会被限制。
func TestListTimeFeedCapsLimit(t *testing.T) {
	repository, mock := newMockRepository(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id AS video_id, author_id, title, media_url, cover_url, like_count, comment_count, favorite_count, published_at FROM "videos" WHERE status = $1 AND published_at IS NOT NULL ORDER BY published_at DESC,id DESC LIMIT $2`)).
		WithArgs(domainvideo.StatusPublished, domainfeed.MaxLimit).
		WillReturnRows(sqlmock.NewRows([]string{"video_id", "author_id", "title", "media_url", "cover_url", "like_count", "comment_count", "favorite_count", "published_at"}))

	if _, err := repository.ListTimeFeed(context.Background(), nil, domainfeed.MaxLimit+1); err != nil {
		t.Fatalf("ListTimeFeed() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

// TestSaveViewEventPersistsEvent 验证观看事件保存后会回写数据库生成的字段。
func TestSaveViewEventPersistsEvent(t *testing.T) {
	repository, mock := newMockRepository(t)
	userID := int64(42)
	event := &domainfeed.ViewEvent{
		UserID:         &userID,
		VisitorID:      "visitor-1",
		VideoID:        7,
		EventType:      domainfeed.EventTypeView,
		WatchMS:        1200,
		IdempotencyKey: "event-1",
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "feed_view_event" ("user_id","visitor_id","video_id","event_type","watch_ms","idempotency_key","created_at") VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING "id"`)).
		WithArgs(userID, event.VisitorID, event.VideoID, event.EventType, event.WatchMS, event.IdempotencyKey, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))
	mock.ExpectCommit()

	if err := repository.SaveViewEvent(context.Background(), event); err != nil {
		t.Fatalf("SaveViewEvent() error = %v", err)
	}
	if event.ID != 9 || event.CreatedAt.IsZero() {
		t.Fatalf("SaveViewEvent() event = %+v, want generated ID and timestamp", event)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

// TestSaveViewEventMapsDuplicateKey 验证观看事件幂等键冲突会转换为领域错误。
func TestSaveViewEventMapsDuplicateKey(t *testing.T) {
	repository, mock := newMockRepository(t)
	event := &domainfeed.ViewEvent{VisitorID: "visitor-1", VideoID: 7, EventType: domainfeed.EventTypeView, IdempotencyKey: "event-1"}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "feed_view_event" ("user_id","visitor_id","video_id","event_type","watch_ms","idempotency_key","created_at") VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING "id"`)).
		WithArgs(nil, event.VisitorID, event.VideoID, event.EventType, event.WatchMS, event.IdempotencyKey, sqlmock.AnyArg()).
		WillReturnError(gorm.ErrDuplicatedKey)
	mock.ExpectRollback()

	if err := repository.SaveViewEvent(context.Background(), event); !errors.Is(err, domainfeed.ErrDuplicateIdempotencyKey) {
		t.Fatalf("SaveViewEvent() error = %v, want %v", err, domainfeed.ErrDuplicateIdempotencyKey)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

// TestFindViewEventByIdempotencyKeyMapsEvent 验证按幂等键查询会恢复完整观看事件。
func TestFindViewEventByIdempotencyKeyMapsEvent(t *testing.T) {
	repository, mock := newMockRepository(t)
	createdAt := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	userID := int64(42)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "feed_view_event" WHERE idempotency_key = $1 ORDER BY "feed_view_event"."id" LIMIT $2`)).
		WithArgs("event-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "visitor_id", "video_id", "event_type", "watch_ms", "idempotency_key", "created_at"}).
			AddRow(9, userID, "visitor-1", 7, "view", 1200, "event-1", createdAt))

	event, err := repository.FindViewEventByIdempotencyKey(context.Background(), " event-1 ")
	if err != nil {
		t.Fatalf("FindViewEventByIdempotencyKey() error = %v", err)
	}
	if event.ID != 9 || event.UserID == nil || *event.UserID != userID || event.EventType != domainfeed.EventTypeView || event.WatchMS != 1200 || !event.CreatedAt.Equal(createdAt) {
		t.Fatalf("FindViewEventByIdempotencyKey() event = %+v, want restored event", event)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

// TestFindViewEventByIdempotencyKeyReturnsNotFound 验证不存在的幂等键会返回领域未找到错误。
func TestFindViewEventByIdempotencyKeyReturnsNotFound(t *testing.T) {
	repository, mock := newMockRepository(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "feed_view_event" WHERE idempotency_key = $1 ORDER BY "feed_view_event"."id" LIMIT $2`)).
		WithArgs("missing", 1).
		WillReturnError(gorm.ErrRecordNotFound)

	if _, err := repository.FindViewEventByIdempotencyKey(context.Background(), "missing"); !errors.Is(err, domainfeed.ErrViewEventNotFound) {
		t.Fatalf("FindViewEventByIdempotencyKey() error = %v, want %v", err, domainfeed.ErrViewEventNotFound)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

// newMockRepository 创建使用 PostgreSQL SQL mock 的信息流仓储。
func newMockRepository(t *testing.T) (*Repository, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	return New(db), mock
}
