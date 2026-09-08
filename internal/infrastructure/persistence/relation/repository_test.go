package infrarelation

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	domainrelation "github.com/inumoroha/video-feed/internal/domain/relation"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestRelationModelsUsePostgresCompatibleStatusType 验证关系状态字段没有使用 MySQL 专用类型。
func TestRelationModelsUsePostgresCompatibleStatusType(t *testing.T) {
	field, ok := reflect.TypeOf(FollowModel{}).FieldByName("Status")
	if !ok {
		t.Fatal("FollowModel is missing Status field")
	}
	if strings.Contains(strings.ToLower(field.Tag.Get("gorm")), "tinyint") {
		t.Fatalf("FollowModel Status GORM tag = %q, must not contain tinyint", field.Tag.Get("gorm"))
	}

	keyField, ok := reflect.TypeOf(FollowIdempotencyModel{}).FieldByName("IdempotencyKey")
	if !ok || !strings.Contains(keyField.Tag.Get("gorm"), "uk_user_idempotency") {
		t.Fatalf("FollowIdempotencyModel IdempotencyKey tag = %q, want user idempotency unique index", keyField.Tag.Get("gorm"))
	}
}

// TestRepositoryRejectsUninitializedAndInvalidInput 验证仓储入口拒绝未初始化实例和无效关系参数。
func TestRepositoryRejectsUninitializedAndInvalidInput(t *testing.T) {
	var uninitialized *Repository
	if _, _, _, err := uninitialized.SetFollow(context.Background(), 1, 2, true, ""); err == nil || err.Error() != "relation repository is not initialized" {
		t.Fatalf("SetFollow() error = %v, want uninitialized repository error", err)
	}

	repository := New(&gorm.DB{})
	if _, _, _, err := repository.SetFollow(context.Background(), 0, 2, true, ""); !errors.Is(err, domainrelation.ErrInvalidUserID) {
		t.Fatalf("SetFollow() error = %v, want %v", err, domainrelation.ErrInvalidUserID)
	}
	if _, _, _, err := repository.SetFollow(context.Background(), 1, 1, true, ""); !errors.Is(err, domainrelation.ErrFollowSelfForbidden) {
		t.Fatalf("SetFollow() error = %v, want %v", err, domainrelation.ErrFollowSelfForbidden)
	}
	if _, _, _, err := repository.SetFollow(context.Background(), 1, 2, true, strings.Repeat("k", domainrelation.MaxIdempotencyKeyLength+1)); !errors.Is(err, domainrelation.ErrIdempotencyKeyTooLong) {
		t.Fatalf("SetFollow() error = %v, want %v", err, domainrelation.ErrIdempotencyKeyTooLong)
	}
	if _, err := repository.ListFollowing(context.Background(), 0, nil, 10); !errors.Is(err, domainrelation.ErrInvalidUserID) {
		t.Fatalf("ListFollowing() error = %v, want %v", err, domainrelation.ErrInvalidUserID)
	}
	if _, err := repository.ListFollowers(context.Background(), 1, nil, 0); !errors.Is(err, domainrelation.ErrInvalidLimit) {
		t.Fatalf("ListFollowers() error = %v, want %v", err, domainrelation.ErrInvalidLimit)
	}
	if _, err := repository.ListFollowing(context.Background(), 1, &domainrelation.ListCursor{UserID: 2}, 10); !errors.Is(err, domainrelation.ErrInvalidCursor) {
		t.Fatalf("ListFollowing() error = %v, want %v", err, domainrelation.ErrInvalidCursor)
	}
}

// TestOrderedUserIDsSortsBothDirections 验证双向关注请求使用相同的用户加锁顺序。
func TestOrderedUserIDsSortsBothDirections(t *testing.T) {
	if got := orderedUserIDs(7, 3); got != [2]int64{3, 7} {
		t.Fatalf("orderedUserIDs(7, 3) = %v, want [3 7]", got)
	}
	if got := orderedUserIDs(3, 7); got != [2]int64{3, 7} {
		t.Fatalf("orderedUserIDs(3, 7) = %v, want [3 7]", got)
	}
}

// TestSetFollowReturnsStoredIdempotencySnapshot 验证相同幂等键重试恢复首次关注状态和双方统计快照。
func TestSetFollowReturnsStoredIdempotencySnapshot(t *testing.T) {
	repository, mock := newRelationMockRepository(t)
	followedAt := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	userStatAt := time.Date(2026, 9, 8, 10, 1, 0, 0, time.UTC)
	targetStatAt := time.Date(2026, 9, 8, 10, 2, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "user_follow_idempotency" WHERE user_id = $1 AND idempotency_key = $2 LIMIT $3 FOR UPDATE`)).
		WithArgs(int64(7), "follow-request-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "idempotency_key", "target_user_id", "follow_id", "active", "follow_created_at", "follow_updated_at",
			"user_following_count", "user_follower_count", "user_stat_created_at", "user_stat_updated_at",
			"target_following_count", "target_follower_count", "target_stat_created_at", "target_stat_updated_at", "created_at",
		}).AddRow(
			1, 7, "follow-request-1", 3, 15, false, followedAt, followedAt,
			8, 2, userStatAt, userStatAt,
			1, 6, targetStatAt, targetStatAt, followedAt,
		))
	mock.ExpectCommit()

	follow, userStat, targetStat, err := repository.SetFollow(context.Background(), 7, 3, true, " follow-request-1 ")
	if err != nil {
		t.Fatalf("SetFollow() error = %v", err)
	}
	if follow == nil || follow.ID != 15 || follow.UserID != 7 || follow.TargetUserID != 3 || follow.Active() || follow.IdempotencyKey != "follow-request-1" {
		t.Fatalf("SetFollow() follow = %#v, want stored canceled relation", follow)
	}
	if userStat == nil || userStat.FollowingCount != 8 || userStat.FollowerCount != 2 {
		t.Fatalf("SetFollow() userStat = %#v, want stored user snapshot", userStat)
	}
	if targetStat == nil || targetStat.FollowingCount != 1 || targetStat.FollowerCount != 6 {
		t.Fatalf("SetFollow() targetStat = %#v, want stored target snapshot", targetStat)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations = %v", err)
	}
}

// TestListQueriesJoinUsersTable 验证关注和粉丝列表查询关联实际的 users 表。
func TestListQueriesJoinUsersTable(t *testing.T) {
	repository, mock := newRelationMockRepositoryWithQueryMatcher(t)
	followedAt := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	mock.ExpectQuery("following").
		WithArgs(int64(7), domainrelation.FollowStatusActive, 1, 10).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "nickname", "avatar_url", "bio", "followed_at"}).
			AddRow(3, "target", "avatar", "bio", followedAt))
	following, err := repository.ListFollowing(context.Background(), 7, nil, 10)
	if err != nil {
		t.Fatalf("ListFollowing() error = %v", err)
	}
	if len(following) != 1 || following[0].UserID != 3 {
		t.Fatalf("ListFollowing() = %#v, want target user", following)
	}

	mock.ExpectQuery("followers").
		WithArgs(int64(7), domainrelation.FollowStatusActive, 1, 10).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "nickname", "avatar_url", "bio", "followed_at"}).
			AddRow(3, "follower", "avatar", "bio", followedAt))
	followers, err := repository.ListFollowers(context.Background(), 7, nil, 10)
	if err != nil {
		t.Fatalf("ListFollowers() error = %v", err)
	}
	if len(followers) != 1 || followers[0].UserID != 3 {
		t.Fatalf("ListFollowers() = %#v, want follower user", followers)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations = %v", err)
	}
}

// newRelationMockRepository 创建使用 PostgreSQL SQL mock 的关系仓储。
func newRelationMockRepository(t *testing.T) (*Repository, sqlmock.Sqlmock) {
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

// newRelationMockRepositoryWithQueryMatcher 创建会校验列表关联表的关系仓储。
func newRelationMockRepositoryWithQueryMatcher(t *testing.T) (*Repository, sqlmock.Sqlmock) {
	t.Helper()
	matcher := sqlmock.QueryMatcherFunc(func(expectedSQL string, actualSQL string) error {
		if expectedSQL != "following" && expectedSQL != "followers" {
			return fmt.Errorf("unexpected query marker %q", expectedSQL)
		}
		if strings.Contains(strings.ToLower(actualSQL), "account") {
			return fmt.Errorf("query must not join account: %s", actualSQL)
		}
		if !strings.Contains(actualSQL, "JOIN users AS a") {
			return fmt.Errorf("query must join users table: %s", actualSQL)
		}
		if expectedSQL == "following" && !strings.Contains(actualSQL, "a.id = f.target_user_id") {
			return fmt.Errorf("following query uses wrong user join: %s", actualSQL)
		}
		if expectedSQL == "followers" && !strings.Contains(actualSQL, "a.id = f.user_id") {
			return fmt.Errorf("followers query uses wrong user join: %s", actualSQL)
		}
		return nil
	})
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
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
