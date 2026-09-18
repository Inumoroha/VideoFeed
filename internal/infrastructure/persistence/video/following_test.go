package infravideo

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"
)

const followerCountSQL = `SELECT "follower_count" FROM "user_relation_stat" WHERE user_id = $1 LIMIT $2`
const fanoutSQL = `INSERT INTO feed_inbox (user_id, video_id, author_id, published_at, created_at)
SELECT f.user_id, $1, $2, $3, NOW()
FROM user_follow AS f
WHERE f.target_user_id = $4 AND f.status = $5
ON CONFLICT (user_id, video_id) DO UPDATE SET published_at = EXCLUDED.published_at`

// TestFanoutThresholdAndDuplicateDelivery 验证粉丝阈值、缺失统计、未发布时间和重复投递使用原子冲突更新。
func TestFanoutThresholdAndDuplicateDelivery(t *testing.T) {
	for _, count := range []int{-1, 0, domainfeed.BigCreatorFollowerThreshold - 1, domainfeed.BigCreatorFollowerThreshold, domainfeed.BigCreatorFollowerThreshold + 1} {
		db, mock := newMockGormDB(t)
		at := time.Now().UTC()
		if err := fanoutPublishedVideoToFollowers(db, 100, 7, nil); err != nil {
			t.Fatal(err)
		}
		for attempt := 0; attempt < 2; attempt++ {
			rows := sqlmock.NewRows([]string{"follower_count"})
			if count >= 0 {
				rows.AddRow(count)
			}
			mock.ExpectQuery(regexp.QuoteMeta(followerCountSQL)).WithArgs(100, 1).WillReturnRows(rows)
			if count < domainfeed.BigCreatorFollowerThreshold {
				mock.ExpectExec(regexp.QuoteMeta(fanoutSQL)).WithArgs(7, 100, at, 100, 1).WillReturnResult(sqlmock.NewResult(0, 2))
			}
			if err := fanoutPublishedVideoToFollowers(db, 100, 7, &at); err != nil {
				t.Fatal(err)
			}
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}
}

// TestSaveFanoutTransactionAndRetry 验证统计或扇出失败时整体回滚，重试成功后才更新领域对象。
func TestSaveFanoutTransactionAndRetry(t *testing.T) {
	for _, stage := range []string{"stat", "count", "inbox", "success"} {
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
				count := mock.ExpectQuery(regexp.QuoteMeta(followerCountSQL)).WithArgs(100, 1)
				if fail && stage == "count" {
					count.WillReturnError(failure)
				} else {
					count.WillReturnRows(sqlmock.NewRows([]string{"follower_count"}).AddRow(2))
					inbox := mock.ExpectExec(regexp.QuoteMeta(fanoutSQL)).WithArgs(7, 100, at, 100, 1)
					if fail {
						inbox.WillReturnError(failure)
					} else {
						inbox.WillReturnResult(sqlmock.NewResult(0, 2))
					}
				}
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
