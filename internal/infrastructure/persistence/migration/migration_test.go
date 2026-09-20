package migration

import (
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"testing"
)

// TestMigrationLockFailure 验证锁获取失败时回滚事务并保留底层错误，空数据库直接失败。
func TestMigrationLockFailure(t *testing.T) {
	if AutoMigrate(nil) == nil {
		t.Fatal("空数据库未校验")
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	orm, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("lock unavailable")
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(736496102\)`).WillReturnError(failure)
	mock.ExpectRollback()
	if err := AutoMigrate(orm); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
