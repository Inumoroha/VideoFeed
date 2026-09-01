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
	if err := repository.SaveTimeCursor(context.TODO(), "cursor", &domainfeed.TimeCursor{}, time.Now().Add(time.Hour)); err == nil || err.Error() != "feed repository is not initialized" {
		t.Fatalf("SaveTimeCursor() error = %v, want uninitialized repository error", err)
	}
	if _, err := repository.FindTimeCursor(context.TODO(), "cursor", time.Now()); err == nil || err.Error() != "feed repository is not initialized" {
		t.Fatalf("FindTimeCursor() error = %v, want uninitialized repository error", err)
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
	if err := repository.SaveTimeCursor(context.TODO(), "", &domainfeed.TimeCursor{}, time.Now().Add(time.Hour)); !errors.Is(err, domainfeed.ErrInvalidCursor) {
		t.Fatalf("SaveTimeCursor() error = %v, want %v", err, domainfeed.ErrInvalidCursor)
	}
	if _, err := repository.FindTimeCursor(context.TODO(), "cursor", time.Time{}); !errors.Is(err, domainfeed.ErrInvalidCursor) {
		t.Fatalf("FindTimeCursor() error = %v, want %v", err, domainfeed.ErrInvalidCursor)
	}
}

// TestListTimeFeedMapsPublishedVideos 验证信息流查询使用正确的视频表并映射返回字段。
func TestListTimeFeedMapsPublishedVideos(t *testing.T) {
	repository, mock := newMockRepository(t)
	publishedAt := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id AS video_id, author_id, title, description, media_url, cover_url, like_count, comment_count, favorite_count, published_at FROM "videos" WHERE status = $1 AND published_at IS NOT NULL ORDER BY published_at DESC,id DESC LIMIT $2`)).
		WithArgs(domainvideo.StatusPublished, 2).
		WillReturnRows(sqlmock.NewRows([]string{"video_id", "author_id", "title", "description", "media_url", "cover_url", "like_count", "comment_count", "favorite_count", "published_at"}).
			AddRow(7, 42, "title", "description", "media", "cover", 3, 4, 5, publishedAt))

	items, err := repository.ListTimeFeed(context.Background(), nil, 2)
	if err != nil {
		t.Fatalf("ListTimeFeed() error = %v", err)
	}
	if len(items) != 1 || items[0].VideoID != 7 || items[0].AuthorID != 42 || items[0].Title != "title" || items[0].Description != "description" || !items[0].PublishedAt.Equal(publishedAt) {
		t.Fatalf("ListTimeFeed() items = %+v, want mapped video item", items)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

// TestListTimeFeedCapsLimit 验证查询数量会保留一条额外记录用于判断是否存在下一页。
func TestListTimeFeedCapsLimit(t *testing.T) {
	repository, mock := newMockRepository(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id AS video_id, author_id, title, description, media_url, cover_url, like_count, comment_count, favorite_count, published_at FROM "videos" WHERE status = $1 AND published_at IS NOT NULL ORDER BY published_at DESC,id DESC LIMIT $2`)).
		WithArgs(domainvideo.StatusPublished, domainfeed.MaxLimit+1).
		WillReturnRows(sqlmock.NewRows([]string{"video_id", "author_id", "title", "description", "media_url", "cover_url", "like_count", "comment_count", "favorite_count", "published_at"}))

	if _, err := repository.ListTimeFeed(context.Background(), nil, domainfeed.MaxLimit+1); err != nil {
		t.Fatalf("ListTimeFeed() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

// TestSaveTimeCursorPersistsCursor 验证分页游标会保存查询位置和过期时间。
func TestSaveTimeCursorPersistsCursor(t *testing.T) {
	repository, mock := newMockRepository(t)
	publishedAt := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	expiresAt := publishedAt.Add(24 * time.Hour)
	cursor := &domainfeed.TimeCursor{PublishedAt: publishedAt, VideoID: 7}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "feed_cursor" ("scene","cursor_token","published_at","video_id","expired_at","created_at") VALUES ($1,$2,$3,$4,$5,$6) RETURNING "id"`)).
		WithArgs(domainfeed.TimeCursorScene, "fc.token", publishedAt, cursor.VideoID, expiresAt, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()

	if err := repository.SaveTimeCursor(context.Background(), " fc.token ", cursor, expiresAt); err != nil {
		t.Fatalf("SaveTimeCursor() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

// TestFindTimeCursorMapsPersistedCursor 验证未过期游标会恢复为时间线查询位置。
func TestFindTimeCursorMapsPersistedCursor(t *testing.T) {
	repository, mock := newMockRepository(t)
	publishedAt := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	now := publishedAt.Add(time.Hour)
	expiresAt := publishedAt.Add(24 * time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "feed_cursor" WHERE scene = $1 AND cursor_token = $2 AND expired_at > $3 ORDER BY "feed_cursor"."id" LIMIT $4`)).
		WithArgs(domainfeed.TimeCursorScene, "fc.token", now, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "scene", "cursor_token", "published_at", "video_id", "expired_at", "created_at"}).
			AddRow(1, domainfeed.TimeCursorScene, "fc.token", publishedAt, 7, expiresAt, publishedAt))

	cursor, err := repository.FindTimeCursor(context.Background(), " fc.token ", now)
	if err != nil {
		t.Fatalf("FindTimeCursor() error = %v", err)
	}
	if cursor.VideoID != 7 || !cursor.PublishedAt.Equal(publishedAt) {
		t.Fatalf("FindTimeCursor() cursor = %+v, want persisted cursor", cursor)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

// TestFindTimeCursorReturnsNotFound 验证未知或已过期的游标会返回未找到错误。
func TestFindTimeCursorReturnsNotFound(t *testing.T) {
	repository, mock := newMockRepository(t)
	now := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "feed_cursor" WHERE scene = $1 AND cursor_token = $2 AND expired_at > $3 ORDER BY "feed_cursor"."id" LIMIT $4`)).
		WithArgs(domainfeed.TimeCursorScene, "missing", now, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	if _, err := repository.FindTimeCursor(context.Background(), "missing", now); !errors.Is(err, domainfeed.ErrTimeCursorNotFound) {
		t.Fatalf("FindTimeCursor() error = %v, want %v", err, domainfeed.ErrTimeCursorNotFound)
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
