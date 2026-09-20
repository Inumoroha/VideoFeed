package recommendation

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	domain "github.com/inumoroha/video-feed/internal/domain/recommendation"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"testing"
	"time"
)

// mockRepository 创建 PostgreSQL 模拟仓储，并在结束时核对预期和释放连接。
func mockRepository(t *testing.T) (*Repository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		_ = db.Close()
	})
	orm, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return New(orm), mock
}

// expectExposure 模拟一次曝光流水、原子聚合和读取，并允许在指定阶段注入故障。
func expectExposure(mock sqlmock.Sqlmock, stage string, now time.Time, failure error) {
	lookup := mock.ExpectQuery(`SELECT "id" FROM "videos" WHERE id = \$1 AND status = \$2 LIMIT \$3`).WithArgs(1, 2, 1)
	if stage == "missing" {
		lookup.WillReturnRows(sqlmock.NewRows([]string{"id"}))
		return
	}
	if stage == "lookup" {
		lookup.WillReturnError(failure)
		return
	}
	lookup.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	insert := mock.ExpectQuery(`INSERT INTO "video_view_events"`).WithArgs(42, 1, "recommend", "req", "exposed", 0, false, sqlmock.AnyArg())
	if stage == "insert" {
		insert.WillReturnError(failure)
		return
	}
	insert.WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(7, now))
	upsert := mock.ExpectQuery(`INSERT INTO "exposures" .*ON CONFLICT \("user_id","video_id"\) DO UPDATE SET "exposure_count"=exposures.exposure_count \+ 1,"last_exposed_at"=EXCLUDED.last_exposed_at,"last_scene"=EXCLUDED.last_scene,"updated_at"=EXCLUDED.updated_at RETURNING "id"`).WithArgs(42, 1, now, now, 1, "recommend", sqlmock.AnyArg(), sqlmock.AnyArg())
	if stage == "upsert" {
		upsert.WillReturnError(failure)
		return
	}
	upsert.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(4))
	read := mock.ExpectQuery(`SELECT \* FROM "exposures" WHERE user_id = \$1 AND video_id = \$2 LIMIT \$3`).WithArgs(42, 1, 1)
	if stage == "read" {
		read.WillReturnError(failure)
		return
	}
	read.WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "video_id", "first_exposed_at", "last_exposed_at", "exposure_count", "last_scene"}).AddRow(4, 42, 1, now.Add(-time.Hour), now, 2, "recommend"))
}

// TestExposureTransactionFailures 验证每个事务阶段的回滚、提交失败、首曝保持及 PostgreSQL 原子更新语句。
func TestExposureTransactionFailures(t *testing.T) {
	for _, stage := range []string{"ok", "missing", "lookup", "insert", "upsert", "read", "commit"} {
		t.Run(stage, func(t *testing.T) {
			r, m := mockRepository(t)
			failure := errors.New("database failure")
			now := time.Now()
			m.ExpectBegin()
			expectExposure(m, stage, now, failure)
			if stage == "ok" {
				m.ExpectCommit()
			} else if stage == "commit" {
				m.ExpectCommit().WillReturnError(failure)
			} else {
				m.ExpectRollback()
			}
			result, err := r.SaveExposures(context.Background(), []*domain.ExposureWrite{{UserID: 42, VideoID: 1, Scene: "recommend", RequestID: "req"}})
			if stage == "ok" {
				if err != nil || len(result) != 1 || result[0].ExposureCount != 2 || !result[0].FirstExposedAt.Equal(now.Add(-time.Hour)) {
					t.Fatal(result, err)
				}
				return
			}
			want := failure
			if stage == "missing" {
				want = domain.ErrVideoNotFound
			}
			if !errors.Is(err, want) || result != nil {
				t.Fatal(result, err)
			}
		})
	}
}

// TestExposureBatchRollbackAndRetry 验证第二条视频不存在会回滚首条写入，修正批次后可显式重试。
func TestExposureBatchRollbackAndRetry(t *testing.T) {
	r, m := mockRepository(t)
	now := time.Now()
	ctx := context.Background()
	write := &domain.ExposureWrite{UserID: 42, VideoID: 1, Scene: "recommend", RequestID: "req"}
	m.ExpectBegin()
	expectExposure(m, "ok", now, nil)
	m.ExpectQuery(`SELECT "id" FROM "videos"`).WithArgs(404, 2, 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	m.ExpectRollback()
	if result, err := r.SaveExposures(ctx, []*domain.ExposureWrite{nil, write, {UserID: 42, VideoID: 404, Scene: "recommend"}}); !errors.Is(err, domain.ErrVideoNotFound) || result != nil {
		t.Fatal(result, err)
	}
	m.ExpectBegin()
	expectExposure(m, "ok", now, nil)
	m.ExpectCommit()
	if _, err := r.SaveExposures(ctx, []*domain.ExposureWrite{write}); err != nil {
		t.Fatal(err)
	}
	if result, err := r.SaveExposures(ctx, nil); err != nil || len(result) != 0 {
		t.Fatal(result, err)
	}
}

// TestCandidatePoolQuery 验证视频表映射、七日曝光排除、状态及热度排序，并传播数据库错误。
func TestCandidatePoolQuery(t *testing.T) {
	r, m := mockRepository(t)
	now := time.Now()
	query := `SELECT v.id AS video_id, v.author_id, .* FROM videos AS v LEFT JOIN video_stat AS vs ON vs.video_id = v.id LEFT JOIN exposures AS e ON e.user_id = \$1 AND e.video_id = v.id AND e.last_exposed_at >= \$2 WHERE v.status = \$3 AND v.published_at IS NOT NULL AND e.video_id IS NULL ORDER BY hot_score DESC,v.published_at DESC,v.id DESC LIMIT \$4`
	m.ExpectQuery(query).WithArgs(42, sqlmock.AnyArg(), 2, 50).WillReturnRows(sqlmock.NewRows([]string{"video_id", "author_id", "hot_score", "published_at"}).AddRow(1, 2, 12, now))
	result, err := r.ListCandidatePool(context.Background(), 42, 50)
	if err != nil || len(result) != 1 || result[0].HotScore != 12 {
		t.Fatal(result, err)
	}
	failure := errors.New("pool")
	m.ExpectQuery(query).WillReturnError(failure)
	if _, err = r.ListCandidatePool(context.Background(), 42, 50); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if result, err = r.ListCandidatePool(context.Background(), 42, 0); err != nil || len(result) != 0 {
		t.Fatal(result, err)
	}
}

// TestVectorsAndInterestRows 验证坏向量与维度不一致跳过、加权画像、结果集关闭及读取故障传播。
func TestVectorsAndInterestRows(t *testing.T) {
	r, m := mockRepository(t)
	ctx := context.Background()
	m.ExpectQuery(`SELECT video_id, embedding_json FROM "video_embedding"`).WillReturnRows(sqlmock.NewRows([]string{"video_id", "embedding_json"}).AddRow(1, "[1,0]").AddRow(2, "broken"))
	vectors, err := r.LoadVideoVectors(ctx, []int64{1, 2})
	if err != nil || len(vectors) != 1 {
		t.Fatal(vectors, err)
	}
	rows := sqlmock.NewRows([]string{"embedding_json", "event_type", "watch_ms", "completed"}).AddRow("[1,0]", "complete", 0, true).AddRow("[0,1]", "play", 0, false).AddRow("broken", "play", 0, false).AddRow("[1,0,0]", "play", 0, false)
	m.ExpectQuery(`SELECT ve.embedding_json, ev.event_type, ev.watch_ms, ev.completed FROM video_view_events AS ev JOIN video_embedding AS ve`).WillReturnRows(rows).RowsWillBeClosed()
	vector, found, err := r.LoadUserInterestVector(ctx, 42)
	if err != nil || !found || vector[0] != 0.75 || vector[1] != 0.25 {
		t.Fatal(vector, found, err)
	}
	failure := errors.New("rows failed")
	m.ExpectQuery(`SELECT ve.embedding_json`).WillReturnRows(sqlmock.NewRows([]string{"embedding_json", "event_type", "watch_ms", "completed"}).AddRow("[1,0]", "play", 0, false).RowError(0, failure)).RowsWillBeClosed()
	if _, _, err = r.LoadUserInterestVector(ctx, 42); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	m.ExpectQuery(`SELECT ve.embedding_json`).WillReturnRows(sqlmock.NewRows([]string{"embedding_json", "event_type", "watch_ms", "completed"})).RowsWillBeClosed()
	if _, found, err = r.LoadUserInterestVector(ctx, 42); err != nil || found {
		t.Fatal(found, err)
	}
	if eventWeight("complete", 0, false) != 3 || eventWeight("play", 90000, true) != 3 || eventWeight("play", 15000, false) != 1.5 || eventWeight("other", 0, false) != 1 {
		t.Fatal("权重错误")
	}
}
