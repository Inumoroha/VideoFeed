package interaction

import (
	"context"
	"errors"
	"testing"
	"time"

	domaininteraction "github.com/inumoroha/video-feed/internal/domain/interaction"
)

// interactionServiceRepository 是服务测试使用的最小内存仓储。
type interactionServiceRepository struct {
	lookupComment   *domaininteraction.Comment
	lookupErr       error
	lookupCalls     int
	listLimit       int
	listComments    []*domaininteraction.Comment
	action          *domaininteraction.Action
	actionErr       error
	likeCount       int
	favoriteCount   int
	setActionCalls  int
	setActionActive bool
	setActionType   string
	deleteComment   *domaininteraction.Comment
	deleteCount     int
	deleteErr       error
	createdComment  *domaininteraction.Comment
}

// SetAction 记录显式互动状态并返回预设的互动动作结果。
func (r *interactionServiceRepository) SetAction(_ context.Context, _ int64, _ int64, actionType string, active bool, _ string) (*domaininteraction.Action, int, int, error) {
	r.setActionCalls++
	r.setActionActive = active
	r.setActionType = actionType
	return r.action, r.likeCount, r.favoriteCount, r.actionErr
}

// CreateComment 返回预设的评论创建结果。
func (r *interactionServiceRepository) CreateComment(_ context.Context, comment *domaininteraction.Comment) (*domaininteraction.Comment, int, error) {
	if r.createdComment != nil {
		return r.createdComment, 1, nil
	}
	return comment, 1, nil
}

// FindCommentByUserAndIdempotencyKey 返回预设的幂等评论结果。
func (r *interactionServiceRepository) FindCommentByUserAndIdempotencyKey(context.Context, int64, string) (*domaininteraction.Comment, int, error) {
	r.lookupCalls++
	return r.lookupComment, 1, r.lookupErr
}

// ListComments 记录服务传入的分页数量并返回预设评论。
func (r *interactionServiceRepository) ListComments(_ context.Context, _ int64, _ *domaininteraction.CommentCursor, limit int) ([]*domaininteraction.Comment, error) {
	r.listLimit = limit
	return r.listComments, nil
}

// DeleteComment 返回预设的评论删除结果。
func (r *interactionServiceRepository) DeleteComment(context.Context, int64, int64, string) (*domaininteraction.Comment, int, error) {
	return r.deleteComment, r.deleteCount, r.deleteErr
}

// GetUserProfile 返回最小用户资料，供不校验触发者的用例满足仓储接口。
func (r *interactionServiceRepository) GetUserProfile(_ context.Context, userID int64) (*domaininteraction.UserProfile, error) {
	return &domaininteraction.UserProfile{ID: userID}, nil
}
// TestServiceRejectsUninitializedDependencies 验证服务依赖缺失时返回明确错误。
func TestServiceRejectsUninitializedDependencies(t *testing.T) {
	var service *Service
	if _, err := service.Like(context.Background(), 1, 1, ""); !errors.Is(err, ErrInteractionServiceNotInitialized) {
		t.Fatalf("Like() error = %v, want %v", err, ErrInteractionServiceNotInitialized)
	}
	if _, err := NewService(nil).ListComments(context.Background(), 1, "", 0); !errors.Is(err, ErrInteractionServiceNotInitialized) {
		t.Fatalf("ListComments() error = %v, want %v", err, ErrInteractionServiceNotInitialized)
	}
}

// TestCreateCommentValidatesBeforeIdempotencyLookup 验证无效评论不会绕过领域校验命中幂等记录。
func TestCreateCommentValidatesBeforeIdempotencyLookup(t *testing.T) {
	repository := &interactionServiceRepository{
		lookupComment: &domaininteraction.Comment{ID: 7},
	}
	service := NewService(repository)

	if _, err := service.CreateComment(context.Background(), 1, 0, "content", "request-1"); !errors.Is(err, domaininteraction.ErrInvalidVideoID) {
		t.Fatalf("CreateComment() error = %v, want %v", err, domaininteraction.ErrInvalidVideoID)
	}
	if repository.lookupCalls != 0 {
		t.Fatalf("CreateComment() lookup calls = %d, want 0", repository.lookupCalls)
	}
}

// TestListCommentsNormalizesLimit 验证负数分页被拒绝且零值使用默认数量。
func TestListCommentsNormalizesLimit(t *testing.T) {
	repository := &interactionServiceRepository{}
	service := NewService(repository)

	if _, err := service.ListComments(context.Background(), 1, "", -1); !errors.Is(err, domaininteraction.ErrInvalidLimit) {
		t.Fatalf("ListComments() error = %v, want %v", err, domaininteraction.ErrInvalidLimit)
	}
	if _, err := service.ListComments(context.Background(), 1, "", 0); err != nil {
		t.Fatalf("ListComments() error = %v, want nil", err)
	}
	if repository.listLimit != defaultCommentLimit+1 {
		t.Fatalf("ListComments() repository limit = %d, want %d", repository.listLimit, defaultCommentLimit+1)
	}
}

// TestLikeReturnsBothActionCounts 验证点赞响应同时包含点赞和收藏统计。
func TestLikeReturnsBothActionCounts(t *testing.T) {
	repository := &interactionServiceRepository{
		action:        &domaininteraction.Action{VideoID: 3, ActionType: domaininteraction.ActionTypeLike, Status: domaininteraction.ActionStatusActive},
		likeCount:     8,
		favoriteCount: 5,
	}
	result, err := NewService(repository).Like(context.Background(), 1, 3, "")
	if err != nil {
		t.Fatalf("Like() error = %v", err)
	}
	if result.LikeCount != 8 || result.FavoriteCount != 5 {
		t.Fatalf("Like() counts = (%d, %d), want (8, 5)", result.LikeCount, result.FavoriteCount)
	}
	if repository.setActionCalls != 1 || !repository.setActionActive || repository.setActionType != domaininteraction.ActionTypeLike {
		t.Fatalf("Like() SetAction call = (calls=%d, active=%t, type=%q), want (1, true, %q)", repository.setActionCalls, repository.setActionActive, repository.setActionType, domaininteraction.ActionTypeLike)
	}
}

// TestUnlikePassesInactiveState 验证取消点赞会向仓储传递 active=false。
func TestUnlikePassesInactiveState(t *testing.T) {
	repository := &interactionServiceRepository{
		action: &domaininteraction.Action{VideoID: 3, ActionType: domaininteraction.ActionTypeLike, Status: domaininteraction.ActionStatusCanceled},
	}
	result, err := NewService(repository).Unlike(context.Background(), 1, 3, "request-1")
	if err != nil {
		t.Fatalf("Unlike() error = %v", err)
	}
	if result.Active {
		t.Fatalf("Unlike() active = %t, want false", result.Active)
	}
	if repository.setActionCalls != 1 || repository.setActionActive || repository.setActionType != domaininteraction.ActionTypeLike {
		t.Fatalf("Unlike() SetAction call = (calls=%d, active=%t, type=%q), want (1, false, %q)", repository.setActionCalls, repository.setActionActive, repository.setActionType, domaininteraction.ActionTypeLike)
	}
}

// TestCommentCursorRoundTrip 验证评论分页游标编码后可以恢复原始位置。
func TestCommentCursorRoundTrip(t *testing.T) {
	want := &domaininteraction.CommentCursor{
		CreatedAt: time.Date(2026, 9, 2, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60)),
		CommentID: 9,
	}
	got, err := parseCommentCursor(encodeCommentCursor(want))
	if err != nil {
		t.Fatalf("parseCommentCursor() error = %v", err)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) || got.CommentID != want.CommentID {
		t.Fatalf("cursor = %+v, want %+v", got, want)
	}
}

var _ domaininteraction.Repository = (*interactionServiceRepository)(nil)
