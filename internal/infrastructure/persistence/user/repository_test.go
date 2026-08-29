package user

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	domainuser "github.com/inumoroha/video-feed/internal/domain/user"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestFindByIDRejectsUninitializedRepository 验证未初始化仓储不会触发空指针异常。
func TestFindByIDRejectsUninitializedRepository(t *testing.T) {
	var repository *Repository
	ctx := context.Background()

	_, err := repository.FindByID(ctx, 1)
	if err == nil || err.Error() != "user repository is not initialized" {
		t.Fatalf("FindByID() error = %v, want uninitialized repository error", err)
	}
}

// TestFindByIDRejectsInvalidID 验证用户 ID 必须为正数。
func TestFindByIDRejectsInvalidID(t *testing.T) {
	repository, _ := NewMockRepository(t)

	_, err := repository.FindByID(context.Background(), 0)
	if !errors.Is(err, domainuser.ErrInvalidUserID) {
		t.Fatalf("FindByID() error = %v, want %v", err, domainuser.ErrInvalidUserID)
	}
}

// TestSaveAndFindByAccountPersistsUser 验证用户新增后可以按账号查询并恢复完整领域对象。
func TestSaveAndFindByAccountPersistsUser(t *testing.T) {
	repository, mock := NewMockRepository(t)
	user := &domainuser.User{
		Account:   "account",
		Password:  "password-hash",
		Nickname:  "nickname",
		Role:      domainuser.RoleUser,
		AvatarURL: "https://example.com/avatar.png",
		Bio:       "profile",
		Status:    domainuser.StatusNormal,
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "users" ("account","password","nickname","role","avatar_url","bio","status","created_at","updated_at") VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING "id"`)).
		WithArgs(user.Account, user.Password, user.Nickname, user.Role, user.AvatarURL, user.Bio, user.Status, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()
	if err := repository.Save(context.Background(), user); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if user.ID <= 0 {
		t.Fatalf("Save() user ID = %d, want positive ID", user.ID)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE account = $1 ORDER BY "users"."id" LIMIT $2`)).
		WithArgs("account", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "account", "password", "nickname", "role", "avatar_url", "bio", "status", "created_at", "updated_at"}).
			AddRow(1, user.Account, user.Password, user.Nickname, user.Role, user.AvatarURL, user.Bio, user.Status, nil, nil))
	got, err := repository.FindByAccount(context.Background(), " account ")
	if err != nil {
		t.Fatalf("FindByAccount() error = %v", err)
	}
	if got.ID != user.ID || got.Account != user.Account || got.Password != user.Password ||
		got.Nickname != user.Nickname || got.Role != user.Role || got.AvatarURL != user.AvatarURL ||
		got.Bio != user.Bio || got.Status != user.Status {
		t.Fatalf("FindByAccount() user = %+v, want %+v", got, user)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

// TestFindByAccountReturnsNotFound 验证查询不存在的账号时返回领域层未找到错误。
func TestFindByAccountReturnsNotFound(t *testing.T) {
	repository, mock := NewMockRepository(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE account = $1 ORDER BY "users"."id" LIMIT $2`)).
		WithArgs("missing", 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := repository.FindByAccount(context.Background(), "missing")
	if !errors.Is(err, domainuser.ErrUserNotFound) {
		t.Fatalf("FindByAccount() error = %v, want %v", err, domainuser.ErrUserNotFound)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

// TestFindByIDReturnsNotFound 验证查询不存在的用户 ID 时返回领域层未找到错误。
func TestFindByIDReturnsNotFound(t *testing.T) {
	repository, mock := NewMockRepository(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE id = $1 ORDER BY "users"."id" LIMIT $2`)).
		WithArgs(1, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := repository.FindByID(context.Background(), 1)
	if !errors.Is(err, domainuser.ErrUserNotFound) {
		t.Fatalf("FindByID() error = %v, want %v", err, domainuser.ErrUserNotFound)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

// TestSaveRejectsDuplicateAccount 验证数据库唯一约束会转换为账号已存在错误。
func TestSaveRejectsDuplicateAccount(t *testing.T) {
	repository, mock := NewMockRepository(t)
	first := &domainuser.User{Account: "account", Password: "hash", Nickname: "first", Role: domainuser.RoleUser, Status: domainuser.StatusNormal}
	second := &domainuser.User{Account: "account", Password: "hash", Nickname: "second", Role: domainuser.RoleUser, Status: domainuser.StatusNormal}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "users" ("account","password","nickname","role","avatar_url","bio","status","created_at","updated_at") VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING "id"`)).
		WithArgs(first.Account, first.Password, first.Nickname, first.Role, first.AvatarURL, first.Bio, first.Status, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()
	if err := repository.Save(context.Background(), first); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "users" ("account","password","nickname","role","avatar_url","bio","status","created_at","updated_at") VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING "id"`)).
		WithArgs(second.Account, second.Password, second.Nickname, second.Role, second.AvatarURL, second.Bio, second.Status, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(gorm.ErrDuplicatedKey)
	mock.ExpectRollback()
	if err := repository.Save(context.Background(), second); !errors.Is(err, domainuser.ErrAccountAlreadyExists) {
		t.Fatalf("second Save() error = %v, want %v", err, domainuser.ErrAccountAlreadyExists)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

// TestUpdateProfileRejectsNilUser 验证更新资料时拒绝空用户对象。
func TestUpdateProfileRejectsNilUser(t *testing.T) {
	repository, _ := NewMockRepository(t)

	err := repository.UpdateProfile(context.TODO(), nil)
	if err == nil || err.Error() != "user is nil" {
		t.Fatalf("UpdateProfile() error = %v, want nil user error", err)
	}
}

// TestUpdateProfileRejectsUninitializedRepository 验证未初始化仓储不会触发空指针异常。
func TestUpdateProfileRejectsUninitializedRepository(t *testing.T) {
	var repository *Repository

	err := repository.UpdateProfile(context.TODO(), &domainuser.User{ID: 1})
	if err == nil || err.Error() != "user repository is not initialized" {
		t.Fatalf("UpdateProfile() error = %v, want uninitialized repository error", err)
	}
}

// TestUpdateProfilePersistsChangedFields 验证资料更新只修改资料字段并可通过查询读取更新结果。
func TestUpdateProfilePersistsChangedFields(t *testing.T) {
	repository, mock := NewMockRepository(t)
	user := &domainuser.User{
		Account:   "account",
		Password:  "password-hash",
		Nickname:  "old nickname",
		Role:      domainuser.RoleUser,
		AvatarURL: "old avatar",
		Bio:       "old bio",
		Status:    domainuser.StatusNormal,
	}
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "users" ("account","password","nickname","role","avatar_url","bio","status","created_at","updated_at") VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING "id"`)).
		WithArgs(user.Account, user.Password, user.Nickname, user.Role, user.AvatarURL, user.Bio, user.Status, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()
	if err := repository.Save(context.Background(), user); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	user.Nickname = "new nickname"
	user.AvatarURL = "new avatar"
	user.Bio = "new bio"

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "users" SET "avatar_url"=$1,"bio"=$2,"nickname"=$3,"updated_at"=$4 WHERE id = $5`)).
		WithArgs(user.AvatarURL, user.Bio, user.Nickname, sqlmock.AnyArg(), user.ID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repository.UpdateProfile(context.TODO(), user); err != nil {
		t.Fatalf("UpdateProfile() error = %v", err)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE id = $1 ORDER BY "users"."id" LIMIT $2`)).
		WithArgs(user.ID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "account", "password", "nickname", "role", "avatar_url", "bio", "status", "created_at", "updated_at"}).
			AddRow(user.ID, user.Account, user.Password, user.Nickname, user.Role, user.AvatarURL, user.Bio, user.Status, nil, nil))
	got, err := repository.FindByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if got.Nickname != "new nickname" || got.AvatarURL != "new avatar" || got.Bio != "new bio" {
		t.Fatalf("FindByID() profile = %+v, want updated fields", got)
	}
	if got.Account != user.Account || got.Password != user.Password || got.Role != user.Role || got.Status != user.Status {
		t.Fatalf("FindByID() unchanged fields = %+v, want account, password, role, and status preserved", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

// TestUpdateProfileReturnsNotFound 验证更新不存在的用户 ID 时返回领域层未找到错误。
func TestUpdateProfileReturnsNotFound(t *testing.T) {
	repository, mock := NewMockRepository(t)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "users" SET "avatar_url"=$1,"bio"=$2,"nickname"=$3,"updated_at"=$4 WHERE id = $5`)).
		WithArgs("", "", "nickname", sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err := repository.UpdateProfile(context.Background(), &domainuser.User{ID: 1, Nickname: "nickname"})
	if !errors.Is(err, domainuser.ErrUserNotFound) {
		t.Fatalf("UpdateProfile() error = %v, want %v", err, domainuser.ErrUserNotFound)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

// NewMockRepository 创建使用 SQL mock 的 PostgreSQL GORM 测试仓储。
func NewMockRepository(t *testing.T) (*Repository, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	return New(db), mock
}
