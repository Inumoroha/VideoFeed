package exposure

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	domainexposure "github.com/inumoroha/video-feed/internal/domain/exposure"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	"sync"
)

// mockRepository 创建使用 PostgreSQL 方言的模拟仓储，并在测试结束核对 SQL 与释放连接。
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

// TestSaveViewEventTransaction 验证 PostgreSQL 原子聚合语句、重复曝光结果、非曝光流水以及各阶段失败回滚。
func TestSaveViewEventTransaction(t *testing.T) {
	failure := errors.New("database unavailable")
	for _, stage := range []string{"exposed", "play", "complete", "skip", "missing", "lookup", "insert", "upsert", "read", "commit"} {
		t.Run(stage, func(t *testing.T) {
			repo, mock := mockRepository(t)
			kind := domainexposure.EventTypeExposed
			if stage == "play" || stage == "complete" || stage == "skip" {
				kind = stage
			}
			event, err := domainexposure.NewViewEvent(42, 1001, "timeline", "", kind, 10, false)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			mock.ExpectBegin()
			lookup := mock.ExpectQuery(regexp.QuoteMeta(`SELECT "id" FROM "videos" WHERE id = $1 AND status = $2 LIMIT $3`)).WithArgs(1001, 2, 1)
			if stage == "missing" {
				lookup.WillReturnRows(sqlmock.NewRows([]string{"id"}))
			} else if stage == "lookup" {
				lookup.WillReturnError(failure)
			} else {
				lookup.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1001))
			}
			if stage != "missing" && stage != "lookup" {
				insert := mock.ExpectQuery(`INSERT INTO "video_view_events"`).WithArgs(42, 1001, "timeline", nil, kind, 10, kind == "complete", sqlmock.AnyArg())
				if stage == "insert" {
					insert.WillReturnError(failure)
				} else {
					insert.WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(8, now))
					if kind == domainexposure.EventTypeExposed {
						upsert := mock.ExpectQuery(`INSERT INTO "exposures" .*ON CONFLICT \("user_id","video_id"\) DO UPDATE SET "exposure_count"=exposures.exposure_count \+ 1,"last_exposed_at"=EXCLUDED.last_exposed_at,"last_scene"=EXCLUDED.last_scene,"updated_at"=EXCLUDED.updated_at RETURNING "id"`).WithArgs(42, 1001, now, now, 1, "timeline", sqlmock.AnyArg(), sqlmock.AnyArg())
						if stage == "upsert" {
							upsert.WillReturnError(failure)
						} else {
							upsert.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
							read := mock.ExpectQuery(`SELECT \* FROM "exposures" WHERE .*user_id = \$1 AND video_id = \$2`).WithArgs(42, 1001, 3, 1)
							if stage == "read" {
								read.WillReturnError(failure)
							} else {
								read.WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "video_id", "first_exposed_at", "last_exposed_at", "exposure_count", "last_scene", "created_at", "updated_at"}).AddRow(3, 42, 1001, now.Add(-time.Hour), now, 2, "timeline", now.Add(-time.Hour), now))
							}
						}
					}
				}
			}
			success := stage == "exposed" || stage == "play" || stage == "complete" || stage == "skip"
			if success {
				mock.ExpectCommit()
			} else if stage == "commit" {
				mock.ExpectCommit().WillReturnError(failure)
			} else {
				mock.ExpectRollback()
			}
			saved, exposure, err := repo.SaveViewEvent(context.Background(), event)
			if !success {
				want := failure
				if stage == "missing" {
					want = domainexposure.ErrVideoNotFound
				}
				if !errors.Is(err, want) || saved != nil || exposure != nil {
					t.Fatalf("saved=%+v exposure=%+v err=%v", saved, exposure, err)
				}
				return
			}
			if err != nil || saved.ID != 8 || saved.RequestID != "" || saved.Completed != (kind == "complete") {
				t.Fatalf("saved=%+v err=%v", saved, err)
			}
			if kind == "exposed" {
				if exposure == nil || exposure.ExposureCount != 2 || !exposure.FirstExposedAt.Equal(now.Add(-time.Hour)) || !exposure.LastExposedAt.Equal(now) {
					t.Fatalf("exposure=%+v", exposure)
				}
			} else if exposure != nil {
				t.Fatal("非曝光事件生成了聚合")
			}
		})
	}
}

// TestExposureSchemaAndErrors 验证用户视频唯一约束、非唯一请求索引、可空请求编号与 PostgreSQL 错误识别。
func TestExposureSchemaAndErrors(t *testing.T) {
	parsed, err := schema.Parse(&ExposureModel{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, index := range parsed.ParseIndexes() {
		if index.Name == "uk_exposures_user_video" {
			found = index.Class == "UNIQUE" && len(index.Fields) == 2 && index.Fields[0].DBName == "user_id" && index.Fields[1].DBName == "video_id"
		}
	}
	if !found {
		t.Fatal("缺失用户视频唯一约束")
	}
	events, err := schema.Parse(&ViewEventModel{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range events.ParseIndexes() {
		if index.Class == "UNIQUE" {
			t.Fatal("观看流水不能增加去重约束")
		}
	}
	if (ViewEventModel{}).TableName() != "video_view_events" || (ExposureModel{}).TableName() != "exposures" {
		t.Fatal("表名变化")
	}
	if stringPtr("") != nil || stringValue(nil) != "" || stringValue(stringPtr("req")) != "req" {
		t.Fatal("可空请求转换错误")
	}
	for _, err := range []error{gorm.ErrDuplicatedKey, &pgconn.PgError{Code: "23505"}} {
		if !isDuplicateKeyError(err) || mapExposureError(err) != err {
			t.Fatal("唯一冲突未保留")
		}
	}
	if isDuplicateKeyError(errors.New("other")) {
		t.Fatal("误判唯一冲突")
	}
}
