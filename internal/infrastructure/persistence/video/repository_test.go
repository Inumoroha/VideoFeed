package infravideo

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestRepositoryRejectsUninitializedRepository 验证未初始化仓储返回错误而非触发空指针异常。
func TestRepositoryRejectsUninitializedRepository(t *testing.T) {
	var repository *Repository

	_, err := repository.FindByID(context.Background(), 1)
	if err == nil || err.Error() != "video repository is not initialized" {
		t.Fatalf("FindByID() error = %v, want uninitialized repository error", err)
	}
}

// TestRepositoryRejectsInvalidInput 验证仓储入口会拒绝无效的领域标识和分页参数。
func TestRepositoryRejectsInvalidInput(t *testing.T) {
	repository, _ := newMockRepository(t)

	if _, err := repository.FindByID(context.TODO(), 0); !errors.Is(err, domainvideo.ErrInvalidVideoID) {
		t.Fatalf("FindByID() error = %v, want %v", err, domainvideo.ErrInvalidVideoID)
	}
	if _, err := repository.FindByIDAnyStatus(context.TODO(), 0); !errors.Is(err, domainvideo.ErrInvalidVideoID) {
		t.Fatalf("FindByIDAnyStatus() error = %v, want %v", err, domainvideo.ErrInvalidVideoID)
	}
	if _, err := repository.FindByAuthorAndIdempotencyKey(context.TODO(), 0, "key"); !errors.Is(err, domainvideo.ErrInvalidAuthorID) {
		t.Fatalf("FindByAuthorAndIdempotencyKey() error = %v, want %v", err, domainvideo.ErrInvalidAuthorID)
	}
	if _, err := repository.ListByAuthor(context.TODO(), 1, 0, 0); !errors.Is(err, domainvideo.ErrInvalidLimit) {
		t.Fatalf("ListByAuthor() error = %v, want %v", err, domainvideo.ErrInvalidLimit)
	}
	if _, err := repository.ListByAuthor(context.TODO(), 1, 1, -1); !errors.Is(err, domainvideo.ErrInvalidOffset) {
		t.Fatalf("ListByAuthor() error = %v, want %v", err, domainvideo.ErrInvalidOffset)
	}
	if err := repository.Save(context.TODO(), nil); err == nil || err.Error() != "video is nil" {
		t.Fatalf("Save() error = %v, want nil video error", err)
	}
	if err := repository.UpdateStatus(context.TODO(), nil); err == nil || err.Error() != "video is nil" {
		t.Fatalf("UpdateStatus() error = %v, want nil video error", err)
	}
}

// TestSaveMapsDuplicateIdempotencyKey 验证唯一键冲突会转换为领域重复幂等键错误。
func TestSaveMapsDuplicateIdempotencyKey(t *testing.T) {
	repository, mock := newMockRepository(t)
	video := &domainvideo.Video{
		AuthorID:       1,
		Title:          "title",
		Description:    "description",
		MediaURL:       "media-url",
		CoverURL:       "cover-url",
		Status:         domainvideo.StatusPublished,
		IdempotencyKey: "key",
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "videos" ("author_id","title","description","media_url","cover_url","status","published_at","idempotency_key","created_at","updated_at") VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING "id"`)).
		WithArgs(video.AuthorID, video.Title, video.Description, video.MediaURL, video.CoverURL, video.Status, nil, "key", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(gorm.ErrDuplicatedKey)
	mock.ExpectRollback()

	if err := repository.Save(context.Background(), video); !errors.Is(err, domainvideo.ErrDuplicateIdempotencyKey) {
		t.Fatalf("Save() error = %v, want %v", err, domainvideo.ErrDuplicateIdempotencyKey)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

// TestFindByIDFiltersUnpublishedVideo 验证公开视频查询仅匹配已发布状态。
func TestFindByIDFiltersUnpublishedVideo(t *testing.T) {
	repository, mock := newMockRepository(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT v.id, v.author_id, v.title, v.description, v.media_url, v.cover_url, v.status, COALESCE(vs.like_count, 0) AS like_count, COALESCE(vs.comment_count, 0) AS comment_count, COALESCE(vs.favorite_count, 0) AS favorite_count, v.published_at, v.idempotency_key, v.created_at, v.updated_at FROM videos AS v LEFT JOIN video_stat AS vs ON vs.video_id = v.id WHERE v.id = $1 AND v.status = $2 LIMIT $3`)).
		WithArgs(1, domainvideo.StatusPublished, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := repository.FindByID(context.Background(), 1)
	if !errors.Is(err, domainvideo.ErrVideoNotFound) {
		t.Fatalf("FindByID() error = %v, want %v", err, domainvideo.ErrVideoNotFound)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

// TestVideoModelAvoidsMySQLSpecificStatusType 验证状态字段未使用 PostgreSQL 不支持的 tinyint 类型。
func TestVideoModelAvoidsMySQLSpecificStatusType(t *testing.T) {
	field, ok := reflect.TypeOf(VideoModel{}).FieldByName("Status")
	if !ok {
		t.Fatal("VideoModel is missing Status field")
	}
	if strings.Contains(field.Tag.Get("gorm"), "tinyint") {
		t.Fatalf("Status GORM tag = %q, must not contain tinyint", field.Tag.Get("gorm"))
	}
}

// TestVideoModelUsesCompatibleDescriptionConstraint 验证描述列长度与领域校验一致且可平滑迁移旧数据。
func TestVideoModelUsesCompatibleDescriptionConstraint(t *testing.T) {
	field, ok := reflect.TypeOf(VideoModel{}).FieldByName("Description")
	if !ok {
		t.Fatal("VideoModel is missing Description field")
	}
	tag := field.Tag.Get("gorm")
	if !strings.Contains(tag, "size:512") || !strings.Contains(tag, "not null") || !strings.Contains(tag, "default:''") {
		t.Fatalf("Description GORM tag = %q, want 512-character non-null field with empty default", tag)
	}
}

// newMockRepository 创建使用 SQL mock 的 PostgreSQL 视频仓储。
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
