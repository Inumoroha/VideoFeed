package infravideo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"
)

// TestSaveTransactionAndRetry 验证统计失败时整体回滚，且不再同步访问粉丝或收件箱，重试成功后才更新领域对象。
func TestSaveTransactionAndRetry(t *testing.T) {
	for _, stage := range []string{"stat", "success"} {
		repo, mock := newMockRepository(t)
		at := time.Now().UTC()
		video := &domainvideo.Video{AuthorID: 100, Title: "test", Status: domainvideo.StatusPublished, PublishedAt: &at, IdempotencyKey: "retry-key"}
		failure := errors.New("injected failure")
		for attempt := 0; attempt < 2; attempt++ {
			fail := attempt == 0 && stage != "success"
			mock.ExpectBegin()
			mock.ExpectQuery(`INSERT INTO "videos"`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
			stat := mock.ExpectQuery(`INSERT INTO "video_stat"`)
			if fail && stage == "stat" {
				stat.WillReturnError(failure)
			} else {
				stat.WillReturnRows(sqlmock.NewRows([]string{"video_id"}).AddRow(7))

			}
			if fail {
				mock.ExpectRollback()
			} else {
				mock.ExpectCommit()
			}
			err := repo.Save(context.Background(), video)
			if fail {
				if !errors.Is(err, failure) || video.ID != 0 {
					t.Fatalf("stage=%s video=%+v err=%v", stage, video, err)
				}
			} else if err != nil || video.ID != 7 {
				t.Fatalf("stage=%s video=%+v err=%v", stage, video, err)
			}
			if stage == "success" {
				break
			}
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}
}
