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
	lookupComment  *domaininteraction.Comment
	lookupErr      error
	lookupCalls    int
	listLimit      int
	listComments   []*domaininteraction.Comment
	toggleAction   *domaininteraction.Action
	toggleCount    int
	toggleErr      error
	deleteComment  *domaininteraction.Comment
	deleteCount    int
	deleteErr      error
	createdComment *domaininteraction.Comment
}

// ToggleAction 返回预设的互动动作结果。
func (r *interactionServiceRepository) ToggleAction(context.Context, int64, int64, string, string) (*domaininteraction.Action, int, error) {
	return r.toggleAction, r.toggleCount, r.toggleErr
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

// TestServiceRejectsUninitializedDependencies 验证服务依赖缺失时返回明确错误。
func TestServiceRejectsUninitializedDependencies(t *testing.T) {
	var service *Service
	if _, err := service.ToggleLike(context.Background(), 1, 1, ""); !errors.Is(err, ErrInteractionServiceNotInitialized) {
		t.Fatalf("ToggleLike() error = %v, want %v", err, ErrInteractionServiceNotInitialized)
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
