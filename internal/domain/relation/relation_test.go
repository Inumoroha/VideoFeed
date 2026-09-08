package relation

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// TestNewFollowCreatesActiveRelation 验证创建关注关系会清理幂等键并设置关注状态。
func TestNewFollowCreatesActiveRelation(t *testing.T) {
	follow, err := NewFollow(1, 2, " request-key ")
	if err != nil {
		t.Fatalf("NewFollow() error = %v", err)
	}
	if follow.UserID != 1 || follow.TargetUserID != 2 || follow.IdempotencyKey != "request-key" || !follow.Active() {
		t.Fatalf("NewFollow() = %+v, want trimmed active relation", follow)
	}
}

// TestNewFollowRejectsInvalidInput 验证创建关注关系会拒绝无效用户、目标用户、自关注和超长幂等键。
func TestNewFollowRejectsInvalidInput(t *testing.T) {
	if _, err := NewFollow(0, 2, ""); !errors.Is(err, ErrInvalidUserID) {
		t.Fatalf("NewFollow() error = %v, want %v", err, ErrInvalidUserID)
	}
	if _, err := NewFollow(1, 0, ""); !errors.Is(err, ErrInvalidTargetUserID) {
		t.Fatalf("NewFollow() error = %v, want %v", err, ErrInvalidTargetUserID)
	}
	if _, err := NewFollow(1, 1, ""); !errors.Is(err, ErrFollowSelfForbidden) {
		t.Fatalf("NewFollow() error = %v, want %v", err, ErrFollowSelfForbidden)
	}
	if _, err := NewFollow(1, 2, strings.Repeat("k", MaxIdempotencyKeyLength+1)); !errors.Is(err, ErrIdempotencyKeyTooLong) {
		t.Fatalf("NewFollow() error = %v, want %v", err, ErrIdempotencyKeyTooLong)
	}
}

// TestFollowSetActiveChangesStatus 验证关注关系可在关注和取关状态之间切换，并安全处理空关系。
func TestFollowSetActiveChangesStatus(t *testing.T) {
	follow := &Follow{Status: FollowStatusActive}
	follow.SetActive(false)
	if follow.Status != FollowStatusCanceled || follow.Active() {
		t.Fatalf("SetActive(false) follow = %+v, want canceled relation", follow)
	}
	follow.SetActive(true)
	if follow.Status != FollowStatusActive || !follow.Active() {
		t.Fatalf("SetActive(true) follow = %+v, want active relation", follow)
	}

	var nilFollow *Follow
	nilFollow.SetActive(false)
	if nilFollow.Active() {
		t.Fatal("nil Follow Active() = true, want false")
	}
}

// TestRestoreFollowNormalizesDefaultStatus 验证恢复关注关系会清理幂等键并为零值状态设置默认关注状态。
func TestRestoreFollowNormalizesDefaultStatus(t *testing.T) {
	follow := RestoreFollow(1, 2, 3, 0, " request-key ", time.Time{}, time.Time{})
	if follow.ID != 1 || follow.UserID != 2 || follow.TargetUserID != 3 || follow.IdempotencyKey != "request-key" || !follow.Active() {
		t.Fatalf("RestoreFollow() = %+v, want normalized active relation", follow)
	}
}

// TestRestoreRelationStatAndUserItemNormalizesValues 验证恢复统计和列表项会归零负数统计并清理展示文本。
func TestRestoreRelationStatAndUserItemNormalizesValues(t *testing.T) {
	stat := RestoreRelationStat(1, -1, 2, time.Time{}, time.Time{})
	if stat.UserID != 1 || stat.FollowingCount != 0 || stat.FollowerCount != 2 {
		t.Fatalf("RestoreRelationStat() = %+v, want non-negative counts", stat)
	}

	item := RestoreUserItem(2, " nickname ", " avatar ", " bio ", time.Time{})
	if item.UserID != 2 || item.Nickname != "nickname" || item.AvatarURL != "avatar" || item.Bio != "bio" {
		t.Fatalf("RestoreUserItem() = %+v, want trimmed display fields", item)
	}
}
