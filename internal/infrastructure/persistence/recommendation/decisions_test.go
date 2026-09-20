package recommendation

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"regexp"
	"testing"
	"time"
)

// TestListRecentExposures 验证按用户、视频集合和含边界时间查询，完整还原字段、空列表短路及错误透传。
func TestListRecentExposures(t *testing.T) {
	repo, mock := mockRepository(t)
	since := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	query := regexp.QuoteMeta(`SELECT * FROM "exposures" WHERE user_id = $1 AND video_id IN ($2,$3) AND last_exposed_at >= $4`)
	mock.ExpectQuery(query).WithArgs(42, 3, 2, since).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "video_id", "first_exposed_at", "last_exposed_at", "exposure_count", "last_scene"}).AddRow(1, 42, 3, since.Add(-time.Hour), since, 2, "timeline"))
	items, err := repo.ListRecentExposures(context.Background(), 42, []int64{3, 2}, since)
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	item := items[0]
	if item.ID != 1 || item.UserID != 42 || item.VideoID != 3 || !item.FirstExposedAt.Equal(since.Add(-time.Hour)) || !item.LastExposedAt.Equal(since) || item.ExposureCount != 2 || item.LastScene != "timeline" {
		t.Fatal(item)
	}
	failure := errors.New("query failed")
	mock.ExpectQuery(query).WithArgs(42, 3, 2, since).WillReturnError(failure)
	if _, err = repo.ListRecentExposures(context.Background(), 42, []int64{3, 2}, since); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if items, err = repo.ListRecentExposures(context.Background(), 42, nil, since); err != nil || items == nil || len(items) != 0 {
		t.Fatal(items, err)
	}
	mock.ExpectQuery(query).WithArgs(42, 3, 2, since).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if items, err = repo.ListRecentExposures(context.Background(), 42, []int64{3, 2}, since); err != nil || items == nil || len(items) != 0 {
		t.Fatal(items, err)
	}
}
