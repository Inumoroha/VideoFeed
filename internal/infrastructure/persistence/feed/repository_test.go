package feed

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"regexp"
	"testing"
	"time"
)

// TestListTimelinePage 验证时间线分页的参数校验、游标和结果映射。
func TestListTimelinePage(t *testing.T) {
	r, m := mockRepo(t)
	at := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	m.ExpectQuery(regexp.QuoteMeta(`SELECT v.id AS video_id, v.published_at FROM videos AS v WHERE v.status = $1 AND v.published_at IS NOT NULL ORDER BY v.published_at DESC,v.id DESC LIMIT $2`)).WithArgs(2, 2).WillReturnRows(sqlmock.NewRows([]string{"video_id", "published_at"}).AddRow(7, at))
	got, err := r.ListTimelinePage(context.Background(), nil, 2)
	if err != nil || len(got) != 1 || got[0].VideoID != 7 {
		t.Fatalf("got=%v err=%v", got, err)
	}
	if err = m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestListHotPage 验证热榜查询按热度和游标稳定排序。
func TestListHotPage(t *testing.T) {
	r, m := mockRepo(t)
	at := time.Now()
	m.ExpectQuery("SELECT .*hot_score.*FROM videos AS v LEFT JOIN video_stat AS vs").WithArgs(2, 2).WillReturnRows(sqlmock.NewRows([]string{"video_id", "hot_score", "published_at"}).AddRow(1, 12, at))
	got, e := r.ListHotPage(context.Background(), nil, 2)
	if e != nil || got[0].HotScore != 12 {
		t.Fatalf("%v %v", got, e)
	}
}

// TestRepositoryValidation 验证仓储和分页参数错误。
func TestRepositoryValidation(t *testing.T) {
	var r *Repository
	if _, e := r.ListTimelinePage(nil, nil, 1); e == nil {
		t.Fatal("want init error")
	}
	rr, _ := mockRepo(t)
	if _, e := rr.ListTimelinePage(context.Background(), nil, 0); !errors.Is(e, domainfeed.ErrInvalidLimit) {
		t.Fatal(e)
	}
}

// mockRepo 创建 PostgreSQL SQL mock 仓储。
func mockRepo(t *testing.T) (*Repository, sqlmock.Sqlmock) {
	db, m, e := sqlmock.New()
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	g, e := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	if e != nil {
		t.Fatal(e)
	}
	return New(g), m
}
