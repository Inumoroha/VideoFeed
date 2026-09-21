package interaction

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	domaininteraction "github.com/inumoroha/video-feed/internal/domain/interaction"
	"gorm.io/gorm"
	"regexp"
	"testing"
	"time"
)

// TestSetActionReplayHasZeroNotificationDelta 验证原幂等快照返回不产生通知变化量。
func TestSetActionReplayHasZeroNotificationDelta(t *testing.T) {
	repository, mock := newInteractionMockRepository(t)
	createdAt := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "interaction_action_idempotency" WHERE user_id = $1 AND action_type = $2 AND idempotency_key = $3 LIMIT $4 FOR UPDATE`)).
		WithArgs(int64(7), domaininteraction.ActionTypeLike, "like-request-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "action_type", "idempotency_key", "video_id", "active", "like_count", "favorite_count", "created_at",
		}).AddRow(1, 7, domaininteraction.ActionTypeLike, "like-request-1", 3, true, 8, 5, createdAt))
	mock.ExpectCommit()

	action, likeCount, favoriteCount, delta, err := repository.SetActionWithDelta(context.Background(), 7, 3, domaininteraction.ActionTypeLike, true, "like-request-1")
	if err != nil {
		t.Fatalf("SetAction() error = %v", err)
	}
	if action == nil || !action.Active() || action.VideoID != 3 {
		t.Fatalf("SetAction() action = %#v, want active action for video 3", action)
	}
	if delta != 0 {
		t.Fatalf("replay delta=%d", delta)
	}
	if likeCount != 8 || favoriteCount != 5 {
		t.Fatalf("SetAction() counts = (%d, %d), want (8, 5)", likeCount, favoriteCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations = %v", err)
	}
}

// TestGetVideoAuthorID 验证通知仅查询已发布视频作者，并将不存在转换为领域错误。
func TestGetVideoAuthorID(t *testing.T) {
	r, m := newInteractionMockRepository(t)
	m.ExpectQuery(`SELECT .* FROM "videos" WHERE id = \$1 AND status = \$2 LIMIT \$3`).WithArgs(int64(1001), 2, 1).WillReturnRows(sqlmock.NewRows([]string{"id", "author_id"}).AddRow(1001, 42))
	if id, err := r.GetVideoAuthorID(context.Background(), 1001); err != nil || id != 42 {
		t.Fatalf("id=%d err=%v", id, err)
	}
	m.ExpectQuery(`SELECT .* FROM "videos"`).WillReturnRows(sqlmock.NewRows([]string{"id", "author_id"}))
	if _, err := r.GetVideoAuthorID(context.Background(), 1002); err != domaininteraction.ErrVideoNotFound {
		t.Fatal(err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestActionDeltaCommitAndRollback 验证实际新增点赞提交后返回正变化量，统计更新失败回滚后变化量归零。
func TestActionDeltaCommitAndRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		r, m := newInteractionMockRepository(t)
		m.ExpectBegin()
		m.ExpectQuery(`SELECT .* FROM "videos"`).WillReturnRows(sqlmock.NewRows([]string{"id", "author_id", "status"}).AddRow(1001, 42, 2))
		m.ExpectQuery(`SELECT .* FROM "interaction_action"`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
		m.ExpectQuery(`INSERT INTO "interaction_action"`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
		m.ExpectQuery(`SELECT .* FROM "video_stat"`).WillReturnRows(sqlmock.NewRows([]string{"video_id", "like_count", "favorite_count"}).AddRow(1001, 0, 0))
		if fail {
			m.ExpectExec(`UPDATE "video_stat"`).WillReturnError(domaininteraction.ErrVideoNotFound)
			m.ExpectRollback()
		} else {
			m.ExpectExec(`UPDATE "video_stat"`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectQuery(`SELECT .* FROM "video_stat"`).WillReturnRows(sqlmock.NewRows([]string{"video_id", "like_count", "favorite_count"}).AddRow(1001, 1, 0))
			m.ExpectCommit()
		}
		action, likes, _, delta, err := r.SetActionWithDelta(context.Background(), 77, 1001, domaininteraction.ActionTypeLike, true, "")
		if fail {
			if err == nil || delta != 0 {
				t.Fatalf("rollback delta=%d err=%v", delta, err)
			}
		} else if err != nil || action == nil || !action.Active() || likes != 1 || delta != 1 {
			t.Fatalf("action=%v likes=%d delta=%d err=%v", action, likes, delta, err)
		}
		if err := m.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}
}

// TestCommentDuplicateHasZeroDelta 验证评论唯一键并发冲突回滚后读取原评论，变化量为零。
func TestCommentDuplicateHasZeroDelta(t *testing.T) {
	r, m := newInteractionMockRepository(t)
	m.ExpectBegin()
	m.ExpectQuery(`SELECT .* FROM "videos"`).WillReturnRows(sqlmock.NewRows([]string{"id", "author_id", "status"}).AddRow(1001, 42, 2))
	m.ExpectQuery(`INSERT INTO "interaction_comment"`).WillReturnError(gorm.ErrDuplicatedKey)
	m.ExpectRollback()
	m.ExpectQuery(`SELECT .* FROM interaction_comment AS c`).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "video_id", "content", "status", "idempotency_key"}).AddRow(5, 77, 1001, "original", 1, "key"))
	m.ExpectQuery(`SELECT .* FROM "video_stat"`).WillReturnRows(sqlmock.NewRows([]string{"video_id", "comment_count"}).AddRow(1001, 1))
	comment, count, delta, err := r.CreateCommentWithDelta(context.Background(), &domaininteraction.Comment{UserID: 77, VideoID: 1001, Content: "retry", IdempotencyKey: "key"})
	if err != nil || comment == nil || comment.ID != 5 || count != 1 || delta != 0 {
		t.Fatalf("comment=%v count=%d delta=%d err=%v", comment, count, delta, err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
