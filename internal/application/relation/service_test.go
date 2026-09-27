package relation

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"testing"
	"time"

	domainrelation "github.com/inumoroha/video-feed/internal/domain/relation"
)

// relationServiceRepository 是关系应用服务测试使用的内存仓储。
type relationServiceRepository struct {
	follow          *domainrelation.Follow
	userStat        *domainrelation.RelationStat
	targetStat      *domainrelation.RelationStat
	setFollowErr    error
	setFollowCalls  int
	setFollowActive bool
	setFollowKey    string
	followingItems  []*domainrelation.UserItem
	followersItems  []*domainrelation.UserItem
	followingErr    error
	followersErr    error
	followingLimit  int
	followersLimit  int
}

// SetFollow 记录应用服务传入的状态和幂等键，并返回预设结果。
func (r *relationServiceRepository) SetFollow(_ context.Context, _ int64, _ int64, active bool, idempotencyKey string) (*domainrelation.Follow, *domainrelation.RelationStat, *domainrelation.RelationStat, error) {
	r.setFollowCalls++
	r.setFollowActive = active
	r.setFollowKey = idempotencyKey
	return r.follow, r.userStat, r.targetStat, r.setFollowErr
}

// ListFollowing 记录关注列表的查询数量，并返回预设结果。
func (r *relationServiceRepository) ListFollowing(_ context.Context, _ int64, _ *domainrelation.ListCursor, limit int) ([]*domainrelation.UserItem, error) {
	r.followingLimit = limit
	return r.followingItems, r.followingErr
}

// ListFollowers 记录粉丝列表的查询数量，并返回预设结果。
func (r *relationServiceRepository) ListFollowers(_ context.Context, _ int64, _ *domainrelation.ListCursor, limit int) ([]*domainrelation.UserItem, error) {
	r.followersLimit = limit
	return r.followersItems, r.followersErr
}

// GetUserProfile 返回可预测的昵称和头像，供关注通知用例断言触发者展示信息。
func (r *relationServiceRepository) GetUserProfile(_ context.Context, userID int64) (*domainrelation.UserProfile, error) {
	return domainrelation.RestoreUserProfile(userID, fmt.Sprintf("viewer-%d", userID), fmt.Sprintf("https://example.com/%d.jpg", userID), ""), nil
}

// TestServiceRejectsUninitializedDependencies 验证服务或仓储未初始化时返回明确错误而不是发生空指针异常。
func TestServiceRejectsUninitializedDependencies(t *testing.T) {
	var service *Service
	if _, err := service.Follow(context.Background(), 1, 2, ""); !errors.Is(err, ErrRelationServiceNotInitialized) {
		t.Fatalf("Follow() error = %v, want %v", err, ErrRelationServiceNotInitialized)
	}
	if _, err := NewService(nil).ListFollowing(context.Background(), 1, "", 0); !errors.Is(err, ErrRelationServiceNotInitialized) {
		t.Fatalf("ListFollowing() error = %v, want %v", err, ErrRelationServiceNotInitialized)
	}
}

// TestFollowNormalizesIdempotencyKeyAndPassesActiveState 验证关注用例清理幂等键并显式传递关注状态。
func TestFollowNormalizesIdempotencyKeyAndPassesActiveState(t *testing.T) {
	repository := &relationServiceRepository{
		follow:     domainrelation.RestoreFollow(1, 7, 3, domainrelation.FollowStatusActive, "request-1", time.Time{}, time.Time{}),
		userStat:   domainrelation.RestoreRelationStat(7, 8, 2, time.Time{}, time.Time{}),
		targetStat: domainrelation.RestoreRelationStat(3, 1, 6, time.Time{}, time.Time{}),
	}

	result, err := NewService(repository).Follow(context.Background(), 7, 3, " request-1 ")
	if err != nil {
		t.Fatalf("Follow() error = %v", err)
	}
	if repository.setFollowCalls != 1 || !repository.setFollowActive || repository.setFollowKey != "request-1" {
		t.Fatalf("SetFollow() call = (calls=%d, active=%t, key=%q), want (1, true, request-1)", repository.setFollowCalls, repository.setFollowActive, repository.setFollowKey)
	}
	if result.UserID != 7 || result.TargetUserID != 3 || !result.Following || result.FollowingCount != 8 || result.FollowerCount != 6 {
		t.Fatalf("Follow() result = %+v, want relation result with both counts", result)
	}
}

// TestUnfollowPassesInactiveState 验证取关用例向仓储显式传递取消关注状态。
func TestUnfollowPassesInactiveState(t *testing.T) {
	repository := &relationServiceRepository{
		follow:     domainrelation.RestoreFollow(1, 7, 3, domainrelation.FollowStatusCanceled, "request-1", time.Time{}, time.Time{}),
		userStat:   domainrelation.RestoreRelationStat(7, 0, 2, time.Time{}, time.Time{}),
		targetStat: domainrelation.RestoreRelationStat(3, 1, 5, time.Time{}, time.Time{}),
	}

	result, err := NewService(repository).Unfollow(context.Background(), 7, 3, "request-1")
	if err != nil {
		t.Fatalf("Unfollow() error = %v", err)
	}
	if repository.setFollowCalls != 1 || repository.setFollowActive {
		t.Fatalf("SetFollow() call = (calls=%d, active=%t), want (1, false)", repository.setFollowCalls, repository.setFollowActive)
	}
	if result.Following || result.FollowingCount != 0 || result.FollowerCount != 5 {
		t.Fatalf("Unfollow() result = %+v, want inactive relation result", result)
	}
}

// TestFollowPreservesUserNotFoundAndRejectsIncompleteRepositoryResult 验证领域未找到错误可透传，缺失结果会转换为应用错误。
func TestFollowPreservesUserNotFoundAndRejectsIncompleteRepositoryResult(t *testing.T) {
	t.Run("user not found", func(t *testing.T) {
		_, err := NewService(&relationServiceRepository{setFollowErr: domainrelation.ErrUserNotFound}).Follow(context.Background(), 7, 3, "")
		if !errors.Is(err, domainrelation.ErrUserNotFound) {
			t.Fatalf("Follow() error = %v, want %v", err, domainrelation.ErrUserNotFound)
		}
	})

	t.Run("nil result", func(t *testing.T) {
		_, err := NewService(&relationServiceRepository{}).Follow(context.Background(), 7, 3, "")
		if !errors.Is(err, ErrUpdateRelationFailed) {
			t.Fatalf("Follow() error = %v, want %v", err, ErrUpdateRelationFailed)
		}
	})
}

// TestListFollowingValidatesPagingAndUsesLookahead 验证列表拒绝负数数量和零值游标，并使用多查一条判断是否有下一页。
func TestListFollowingValidatesPagingAndUsesLookahead(t *testing.T) {
	repository := &relationServiceRepository{
		followingItems: []*domainrelation.UserItem{
			{UserID: 3, FollowedAt: time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC)},
			{UserID: 2, FollowedAt: time.Date(2026, 9, 9, 9, 0, 0, 0, time.UTC)},
		},
	}
	service := NewService(repository)
	if _, err := service.ListFollowing(context.Background(), 7, "", -1); !errors.Is(err, domainrelation.ErrInvalidLimit) {
		t.Fatalf("ListFollowing() error = %v, want %v", err, domainrelation.ErrInvalidLimit)
	}
	if repository.followingLimit != 0 {
		t.Fatalf("ListFollowing() queried with invalid limit = %d, want no repository call", repository.followingLimit)
	}

	zeroCursor := base64.RawURLEncoding.EncodeToString([]byte(`{"followed_at":"0001-01-01T00:00:00Z","user_id":3}`))
	if _, err := service.ListFollowing(context.Background(), 7, zeroCursor, 1); !errors.Is(err, domainrelation.ErrInvalidCursor) {
		t.Fatalf("ListFollowing() error = %v, want %v", err, domainrelation.ErrInvalidCursor)
	}

	result, err := service.ListFollowing(context.Background(), 7, "", 1)
	if err != nil {
		t.Fatalf("ListFollowing() error = %v", err)
	}
	if repository.followingLimit != 2 || len(result.Items) != 1 || !result.HasMore || result.NextCursor == "" {
		t.Fatalf("ListFollowing() result = %+v, limit = %d, want one item with next cursor", result, repository.followingLimit)
	}
}

// TestListFollowersMapsExpectedDomainError 验证粉丝列表会保留仓储返回的领域错误。
func TestListFollowersMapsExpectedDomainError(t *testing.T) {
	_, err := NewService(&relationServiceRepository{followersErr: domainrelation.ErrUserNotFound}).ListFollowers(context.Background(), 7, "", 10)
	if !errors.Is(err, domainrelation.ErrUserNotFound) {
		t.Fatalf("ListFollowers() error = %v, want %v", err, domainrelation.ErrUserNotFound)
	}
}

var _ domainrelation.Repository = (*relationServiceRepository)(nil)
