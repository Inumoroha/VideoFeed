package feed

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	"regexp"
	"testing"
)

// TestFanoutRepositoryQueries 验证大作者阈值、全部关注作者、有效粉丝分页、缺失统计和错误传播。
func TestFanoutRepositoryQueries(t *testing.T) {
	r, m := mockRepo(t)
	ctx := context.Background()
	m.ExpectQuery(regexp.QuoteMeta(`SELECT f.target_user_id FROM user_follow AS f JOIN user_relation_stat AS rs ON rs.user_id = f.target_user_id WHERE f.user_id = $1 AND f.status = $2 AND rs.follower_count >= $3 ORDER BY f.target_user_id ASC`)).WithArgs(42, 1, 10000).WillReturnRows(sqlmock.NewRows([]string{"target_user_id"}).AddRow(7))
	ids, err := r.ListFollowingPullAuthorIDs(ctx, 42)
	if err != nil || len(ids) != 1 || ids[0] != 7 {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
	m.ExpectQuery(regexp.QuoteMeta(`SELECT target_user_id FROM "user_follow" WHERE user_id = $1 AND status = $2 ORDER BY target_user_id ASC`)).WithArgs(42, 1).WillReturnRows(sqlmock.NewRows([]string{"target_user_id"}).AddRow(7).AddRow(8))
	ids, err = r.ListFollowingAuthorIDs(ctx, 42)
	if err != nil || len(ids) != 2 {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
	for _, count := range []int{-1, 9999, 10000} {
		rows := sqlmock.NewRows([]string{"follower_count"})
		if count >= 0 {
			rows.AddRow(count)
		}
		m.ExpectQuery(regexp.QuoteMeta(`SELECT "follower_count" FROM "user_relation_stat" WHERE user_id = $1 LIMIT $2`)).WithArgs(7, 1).WillReturnRows(rows)
		got, err := r.CountFollowers(ctx, 7)
		want := count
		if want < 0 {
			want = 0
		}
		if err != nil || got != want {
			t.Fatalf("count=%d err=%v", got, err)
		}
	}
	m.ExpectQuery(regexp.QuoteMeta(`SELECT user_id FROM "user_follow" WHERE (target_user_id = $1 AND status = $2) AND user_id > $3 ORDER BY user_id ASC LIMIT $4`)).WithArgs(7, 1, 10, 2).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(11).AddRow(12))
	ids, err = r.ListFollowerIDs(ctx, 7, 10, 2)
	if err != nil || len(ids) != 2 || ids[0] != 11 {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
	failure := errors.New("database down")
	m.ExpectQuery("SELECT .*follower_count").WillReturnError(failure)
	if _, err := r.CountFollowers(ctx, 7); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	m.ExpectQuery("SELECT .*user_id.*FROM.*user_follow").WillReturnError(failure)
	if _, err := r.ListFollowerIDs(ctx, 7, 0, 2); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestValidateFollowingIndex 验证批量可见性查询过滤删除状态及失效关注关系。
func TestValidateFollowingIndex(t *testing.T) {
	r, m := mockRepo(t)
	ctx := context.Background()
	items := []*domainfeed.FeedPageItem{{VideoID: 1}, {VideoID: 2}}
	for _, count := range []int{2, 1} {
		m.ExpectQuery(`SELECT count\(\*\) FROM videos AS v JOIN user_follow AS f ON f.target_user_id = v.author_id WHERE v.id IN \(\$1,\$2\) AND v.status = \$3 AND v.published_at IS NOT NULL AND f.user_id = \$4 AND f.status = \$5`).WithArgs(1, 2, 2, 42, 1).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
		valid, err := r.ValidateFollowingIndex(ctx, 42, items)
		if err != nil || valid != (count == 2) {
			t.Fatalf("valid=%v err=%v", valid, err)
		}
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
