package user

import (
	"context"
	"errors"
	"testing"

	domainuser "github.com/inumoroha/video-feed/internal/domain/user"
	"gorm.io/gorm"
)

// TestFindByIDRejectsUninitializedRepository 验证未初始化仓储不会触发空指针异常。
func TestFindByIDRejectsUninitializedRepository(t *testing.T) {
	var repository *Repository

	_, err := repository.FindByID(nil, 1)
	if err == nil || err.Error() != "user repository is not initialized" {
		t.Fatalf("FindByID() error = %v, want uninitialized repository error", err)
	}
}

// TestFindByIDRejectsInvalidID 验证用户 ID 必须为正数。
func TestFindByIDRejectsInvalidID(t *testing.T) {
	repository := New(&gorm.DB{})

	_, err := repository.FindByID(context.Background(), 0)
	if !errors.Is(err, domainuser.ErrInvalidUserID) {
		t.Fatalf("FindByID() error = %v, want %v", err, domainuser.ErrInvalidUserID)
	}
}

// TestUpdateProfileRejectsNilUser 验证更新资料时拒绝空用户对象。
func TestUpdateProfileRejectsNilUser(t *testing.T) {
	repository := New(&gorm.DB{})

	err := repository.UpdateProfile(nil, nil)
	if err == nil || err.Error() != "user is nil" {
		t.Fatalf("UpdateProfile() error = %v, want nil user error", err)
	}
}

// TestUpdateProfileRejectsUninitializedRepository 验证未初始化仓储不会触发空指针异常。
func TestUpdateProfileRejectsUninitializedRepository(t *testing.T) {
	var repository *Repository

	err := repository.UpdateProfile(nil, &domainuser.User{ID: 1})
	if err == nil || err.Error() != "user repository is not initialized" {
		t.Fatalf("UpdateProfile() error = %v, want uninitialized repository error", err)
	}
}
