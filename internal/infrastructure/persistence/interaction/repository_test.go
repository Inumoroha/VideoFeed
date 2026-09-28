package interaction

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	domaininteraction "github.com/inumoroha/video-feed/internal/domain/interaction"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestInteractionModelsUsePostgresCompatibleStatusType 验证互动模型未使用 MySQL 专用状态类型。
func TestInteractionModelsUsePostgresCompatibleStatusType(t *testing.T) {
	for _, model := range []any{ActionModel{}, CommentModel{}} {
		field, ok := reflect.TypeOf(model).FieldByName("Status")
		if !ok {
			t.Fatalf("%T is missing Status field", model)
		}
		if strings.Contains(strings.ToLower(field.Tag.Get("gorm")), "tinyint") {
			t.Fatalf("%T Status GORM tag = %q, must not contain tinyint", model, field.Tag.Get("gorm"))
		}
	}
}

// TestRepositoryRejectsUninitializedRepository 验证未初始化仓储不会触发空指针异常。
func TestRepositoryRejectsUninitializedRepository(t *testing.T) {
	var repository *Repository
	if _, _, _, err := repository.SetAction(context.Background(), 1, 1, domaininteraction.ActionTypeLike, true, ""); err == nil || err.Error() != "interaction repository is not initialized" {
		t.Fatalf("SetAction() error = %v, want uninitialized repository error", err)
	}
	if _, err := repository.FindCommentByID(context.Background(), 1); err == nil || err.Error() != "interaction repository is not initialized" {
		t.Fatalf("FindCommentByID() error = %v, want uninitialized repository error", err)
	}
}

// TestRepositoryRejectsInvalidInput 验证仓储入口会拒绝无效标识、分页参数和游标。
func TestRepositoryRejectsInvalidInput(t *testing.T) {
	repository := New(&gorm.DB{})

	if _, _, _, err := repository.SetAction(context.Background(), 0, 1, domaininteraction.ActionTypeLike, true, ""); !errors.Is(err, domaininteraction.ErrInvalidUserID) {
		t.Fatalf("SetAction() error = %v, want %v", err, domaininteraction.ErrInvalidUserID)
	}
	if _, err := repository.FindCommentByID(context.Background(), 0); !errors.Is(err, domaininteraction.ErrInvalidCommentID) {
		t.Fatalf("FindCommentByID() error = %v, want %v", err, domaininteraction.ErrInvalidCommentID)
	}
	if _, err := repository.ListComments(context.Background(), 1, nil, 0); !errors.Is(err, domaininteraction.ErrInvalidLimit) {
		t.Fatalf("ListComments() error = %v, want %v", err, domaininteraction.ErrInvalidLimit)
	}
	if _, err := repository.ListComments(context.Background(), 1, &domaininteraction.CommentCursor{CommentID: 1}, 10); !errors.Is(err, domaininteraction.ErrInvalidCursor) {
		t.Fatalf("ListComments() error = %v, want %v", err, domaininteraction.ErrInvalidCursor)
	}
}

// TestValidateCommentCountsUnicodeCharacters 验证持久化入口与领域层一致地按字符校验评论长度。
func TestValidateCommentCountsUnicodeCharacters(t *testing.T) {
	comment := &domaininteraction.Comment{
		VideoID: 1,
		UserID:  2,
		Content: strings.Repeat("中", domaininteraction.MaxCommentContentLength),
	}
	if err := validateComment(comment); err != nil {
		t.Fatalf("validateComment() error = %v", err)
	}
	comment.Content += "中"
	if !errors.Is(validateComment(comment), domaininteraction.ErrCommentContentTooLong) {
		t.Fatalf("validateComment() error = %v, want %v", validateComment(comment), domaininteraction.ErrCommentContentTooLong)
	}
}

// TestSetActionReturnsStoredIdempotencySnapshot 验证相同幂等键重试返回首次执行的动作状态和统计快照。
func TestSetActionReturnsStoredIdempotencySnapshot(t *testing.T) {
	repository, mock := newInteractionMockRepository(t)
	createdAt := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "interaction_action_idempotency" WHERE user_id = $1 AND action_type = $2 AND idempotency_key = $3 LIMIT $4 FOR UPDATE`)).
		WithArgs(int64(7), domaininteraction.ActionTypeLike, "like-request-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "action_type", "idempotency_key", "video_id", "active", "like_count", "favorite_count", "created_at",
		}).AddRow(1, 7, domaininteraction.ActionTypeLike, "like-request-1", 3, true, 8, 5, createdAt))
	mock.ExpectCommit()

	action, likeCount, favoriteCount, err := repository.SetAction(context.Background(), 7, 3, domaininteraction.ActionTypeLike, true, "like-request-1")
	if err != nil {
		t.Fatalf("SetAction() error = %v", err)
	}
	if action == nil || !action.Active() || action.VideoID != 3 {
		t.Fatalf("SetAction() action = %#v, want active action for video 3", action)
	}
	if likeCount != 8 || favoriteCount != 5 {
		t.Fatalf("SetAction() counts = (%d, %d), want (8, 5)", likeCount, favoriteCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations = %v", err)
	}
}

// TestResolveActionTransitionFollowsRequestedState 验证状态与变化量按请求的 active 直接决定，而不是按当前状态取反。
func TestResolveActionTransitionFollowsRequestedState(t *testing.T) {
	for _, tc := range []struct {
		name          string
		currentStatus int
		active        bool
		wantStatus    int
		wantDelta     int
	}{
		{"取消点赞且当前有效", domaininteraction.ActionStatusActive, false, domaininteraction.ActionStatusCanceled, -1},
		{"取消点赞且当前已取消", domaininteraction.ActionStatusCanceled, false, domaininteraction.ActionStatusCanceled, 0},
		{"点赞且当前已取消", domaininteraction.ActionStatusCanceled, true, domaininteraction.ActionStatusActive, 1},
		{"点赞且当前有效", domaininteraction.ActionStatusActive, true, domaininteraction.ActionStatusActive, 0},
		{"取消收藏且当前有效", domaininteraction.ActionStatusActive, false, domaininteraction.ActionStatusCanceled, -1},
		{"零值状态视为有效且取消请求按切换处理", 0, false, domaininteraction.ActionStatusCanceled, -1},
		{"零值状态视为有效且点赞请求不重复计数", 0, true, domaininteraction.ActionStatusActive, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotStatus, gotDelta := resolveActionTransition(tc.currentStatus, tc.active)
			if gotStatus != tc.wantStatus || gotDelta != tc.wantDelta {
				t.Fatalf("resolveActionTransition(%d, %t) = (%d, %d), want (%d, %d)",
					tc.currentStatus, tc.active, gotStatus, gotDelta, tc.wantStatus, tc.wantDelta)
			}
		})
	}
}

// TestResolveActionTransitionIsIdempotentPerState 验证同一目标状态连续请求只会产生一次计数变化。
func TestResolveActionTransitionIsIdempotentPerState(t *testing.T) {
	status := domaininteraction.ActionStatusCanceled
	totalDelta := 0

	status, delta := resolveActionTransition(status, true)
	totalDelta += delta
	if status != domaininteraction.ActionStatusActive || delta != 1 {
		t.Fatalf("first like = (%d, %d), want (%d, 1)", status, delta, domaininteraction.ActionStatusActive)
	}
	if _, delta = resolveActionTransition(status, true); delta != 0 {
		t.Fatalf("repeated like delta = %d, want 0", delta)
	}
	status, delta = resolveActionTransition(status, false)
	totalDelta += delta
	if status != domaininteraction.ActionStatusCanceled || delta != -1 {
		t.Fatalf("unlike = (%d, %d), want (%d, -1)", status, delta, domaininteraction.ActionStatusCanceled)
	}
	if _, delta = resolveActionTransition(status, false); delta != 0 {
		t.Fatalf("repeated unlike delta = %d, want 0", delta)
	}
	if totalDelta != 0 {
		t.Fatalf("like then unlike net delta = %d, want 0", totalDelta)
	}
}

// newInteractionMockRepository 创建使用 PostgreSQL SQL mock 的互动仓储。
func newInteractionMockRepository(t *testing.T) (*Repository, sqlmock.Sqlmock) {
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
