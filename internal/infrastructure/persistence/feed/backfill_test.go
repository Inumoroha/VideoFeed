package feed

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"regexp"
	"testing"
	"time"
)

// TestListAuthorRecentVideos 验证目标表名、已发布过滤、稳定排序、数量限制、空入参及数据库故障。
func TestListAuthorRecentVideos(t *testing.T) {
	r, m := mockRepo(t)
	ctx := context.Background()
	for _, args := range [][2]int64{{0, 100}, {77, 0}} {
		items, err := r.ListAuthorRecentVideos(ctx, args[0], int(args[1]))
		if err != nil || len(items) != 0 {
			t.Fatal(items, err)
		}
	}
	query := regexp.QuoteMeta(`SELECT v.id AS video_id, v.author_id, v.published_at FROM videos AS v WHERE v.author_id = $1 AND v.status = $2 AND v.published_at IS NOT NULL ORDER BY v.published_at DESC,v.id DESC LIMIT $3`)
	at := time.Now()
	m.ExpectQuery(query).WithArgs(77, 2, 100).WillReturnRows(sqlmock.NewRows([]string{"video_id", "author_id", "published_at"}).AddRow(101, 77, at).AddRow(100, 77, at))
	items, err := r.ListAuthorRecentVideos(ctx, 77, 100)
	if err != nil || len(items) != 2 || items[0].VideoID != 101 || items[1].VideoID != 100 || items[0] == items[1] {
		t.Fatal(items, err)
	}
	failure := errors.New("db")
	m.ExpectQuery(query).WillReturnError(failure)
	if _, err = r.ListAuthorRecentVideos(ctx, 77, 100); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err = m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
