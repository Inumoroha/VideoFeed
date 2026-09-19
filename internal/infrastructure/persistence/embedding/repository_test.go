package embedding

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	domainembedding "github.com/inumoroha/video-feed/internal/domain/embedding"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// mockRepository 创建 PostgreSQL 方言的模拟仓储，结束时验证 SQL 并关闭连接。
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

// TestSaveVideoEmbeddingSQL 验证复合冲突目标、可更新字段及写入失败回滚和错误透传。
func TestSaveVideoEmbeddingSQL(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "rollback"}[failed], func(t *testing.T) {
			repo, mock := mockRepository(t)
			value := domainembedding.NewVideoEmbedding(1, "model", []float64{1}, "hash", "[1]")
			failure := errors.New("database failed")
			mock.ExpectBegin()
			insert := mock.ExpectExec(`INSERT INTO "video_embedding" .* ON CONFLICT \("video_id","model"\) DO UPDATE SET "dimension"="excluded"\."dimension","embedding_json"="excluded"\."embedding_json","text_hash"="excluded"\."text_hash","updated_at"="excluded"\."updated_at"`).WithArgs(1, "model", 1, "[1]", "hash", sqlmock.AnyArg(), sqlmock.AnyArg())
			if failed {
				insert.WillReturnError(failure)
				mock.ExpectRollback()
			} else {
				insert.WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			err := repo.SaveVideoEmbedding(context.Background(), value)
			if failed && !errors.Is(err, failure) || !failed && err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestFindVideoEmbeddingSQL 验证字段恢复、不解析 JSON、缺失错误映射及数据库错误透传。
func TestFindVideoEmbeddingSQL(t *testing.T) {
	for _, state := range []string{"found", "missing", "error"} {
		t.Run(state, func(t *testing.T) {
			repo, mock := mockRepository(t)
			now := time.Now()
			failure := errors.New("query failed")
			query := mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "video_embedding" WHERE video_id = $1 AND model = $2 LIMIT $3`)).WithArgs(1, "model", 1)
			rows := sqlmock.NewRows([]string{"video_id", "model", "dimension", "embedding_json", "text_hash", "created_at", "updated_at"})
			if state == "found" {
				rows.AddRow(1, "model", 1, "[1]", "hash", now, now)
			}
			if state == "error" {
				query.WillReturnError(failure)
			} else {
				query.WillReturnRows(rows)
			}
			value, err := repo.FindVideoEmbedding(context.Background(), 1, "model")
			switch state {
			case "found":
				if err != nil || value.VideoID != 1 || value.Model != "model" || value.Dimension != 1 || value.EmbeddingJSON != "[1]" || value.TextHash != "hash" || value.Embedding != nil || !value.CreatedAt.Equal(now) || !value.UpdatedAt.Equal(now) {
					t.Fatal(value, err)
				}
			case "missing":
				if err != domainembedding.ErrVideoEmbeddingNotFound || value != nil {
					t.Fatal(value, err)
				}
			case "error":
				if !errors.Is(err, failure) || value != nil {
					t.Fatal(value, err)
				}
			}
		})
	}
}
