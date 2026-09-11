package feed

import (
	"context"
	"errors"
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

	if _, err := repository.ListTimelineFeed(context.TODO(), nil, 10); err == nil || err.Error() != "feed repository is not initialized" {
		t.Fatalf("ListTimelineFeed() error = %v, want uninitialized repository error", err)
	}
}

// TestRepositoryRejectsInvalidInput 验证仓储会拒绝非法分页大小和游标。
func TestRepositoryRejectsInvalidInput(t *testing.T) {
	repository, _ := newMockRepository(t)

	if _, err := repository.ListTimelineFeed(context.TODO(), nil, 0); !errors.Is(err, domainfeed.ErrInvalidLimit) {
		t.Fatalf("ListTimelineFeed() error = %v, want %v", err, domainfeed.ErrInvalidLimit)
	}

	if _, err := repository.ListTimelineFeed(context.TODO(), &domainfeed.TimelineCursor{VideoID: 1}, 10); !errors.Is(err, domainfeed.ErrInvalidCursor) {
		t.Fatalf("ListTimelineFeed() error = %v, want %v", err, domainfeed.ErrInvalidCursor)
	}
}

// TestListTimelineFeedMapsPublishedVideos 验证时间线查询会映射视频、作者和互动字段。
func TestListTimelineFeedMapsPublishedVideos(t *testing.T) {
	repository, mock := newMockRepository(t)
	publishedAt := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(timelineQueryPattern).
		WithArgs(domainvideo.StatusPublished, 2).
		WillReturnRows(sqlmock.NewRows([]string{
			"video_id", "author_id", "author_nickname", "author_avatar_url",
			"title", "description", "media_url", "cover_url",
			"like_count", "comment_count", "favorite_count", "published_at",
		}).AddRow(
			7, 42, " author ", " avatar ", " title ", " description ",
			" media ", " cover ", 3, 4, 5, publishedAt,
		))

	items, err := repository.ListTimelineFeed(context.Background(), nil, 2)
	if err != nil {
		t.Fatalf("ListTimelineFeed() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("ListTimelineFeed() item count = %d, want 1", len(items))
	}

	item := items[0]
	if item.VideoID != 7 || item.AuthorID != 42 || item.AuthorNickname != "author" || item.AuthorAvatarURL != "avatar" {
		t.Fatalf("ListTimelineFeed() identity = %+v, want mapped identity", item)
	}
	if item.Title != "title" || item.Description != "description" || item.MediaURL != "media" || item.CoverURL != "cover" {
		t.Fatalf("ListTimelineFeed() content = %+v, want mapped content", item)
	}
	if item.LikeCount != 3 || item.CommentCount != 4 || item.FavoriteCount != 5 || !item.PublishedAt.Equal(publishedAt) {
		t.Fatalf("ListTimelineFeed() metadata = %+v, want mapped metadata", item)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

// TestListTimelineFeedCapsLimit 验证查询数量最多保留一条额外记录用于判断下一页。
func TestListTimelineFeedCapsLimit(t *testing.T) {
	repository, mock := newMockRepository(t)
	mock.ExpectQuery(timelineQueryPattern).
		WithArgs(domainvideo.StatusPublished, domainfeed.MaxLimit+1).
		WillReturnRows(sqlmock.NewRows([]string{
			"video_id", "author_id", "author_nickname", "author_avatar_url",
			"title", "description", "media_url", "cover_url",
			"like_count", "comment_count", "favorite_count", "published_at",
		}))

	if _, err := repository.ListTimelineFeed(context.Background(), nil, domainfeed.MaxLimit+100); err != nil {
		t.Fatalf("ListTimelineFeed() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

// TestListTimelineFeedAppliesCursor 验证查询会使用发布时间和视频 ID 过滤后续页面。
func TestListTimelineFeedAppliesCursor(t *testing.T) {
	repository, mock := newMockRepository(t)
	publishedAt := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	cursor := &domainfeed.TimelineCursor{PublishedAt: publishedAt, VideoID: 7}
	mock.ExpectQuery(timelineQueryPattern).
		WithArgs(domainvideo.StatusPublished, publishedAt, publishedAt, cursor.VideoID, 2).
		WillReturnRows(sqlmock.NewRows([]string{
			"video_id", "author_id", "author_nickname", "author_avatar_url",
			"title", "description", "media_url", "cover_url",
			"like_count", "comment_count", "favorite_count", "published_at",
		}))

	if _, err := repository.ListTimelineFeed(context.Background(), cursor, 2); err != nil {
		t.Fatalf("ListTimelineFeed() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

const timelineQueryPattern = `(?s)SELECT .*video_id.*a\.nickname AS author_nickname.*a\.avatar_url AS author_avatar_url.*hot_score.*FROM videos AS v LEFT JOIN users AS a ON a\.id = v\.author_id LEFT JOIN video_stat AS vs ON vs\.video_id = v\.id WHERE .*v\.status = \$[0-9]+.*v\.published_at IS NOT NULL.*ORDER BY v\.published_at DESC,v\.id DESC LIMIT \$[0-9]+`

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
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	return New(db), mock
}
