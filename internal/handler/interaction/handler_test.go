package interaction

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	applicationinteraction "github.com/inumoroha/video-feed/internal/application/interaction"
	domaininteraction "github.com/inumoroha/video-feed/internal/domain/interaction"
	"github.com/inumoroha/video-feed/internal/middleware"
)

// interactionHandlerRepository 是互动 handler 测试使用的内存仓储替身。
type interactionHandlerRepository struct {
	action        *domaininteraction.Action
	likeCount     int
	favoriteCount int
	setActionType string
	setActionOn   bool
	deleteComment *domaininteraction.Comment
	deleteUserID  int64
	deleteRole    string
}

// SetAction 记录显式互动状态并返回预设的互动动作。
func (r *interactionHandlerRepository) SetAction(_ context.Context, _ int64, _ int64, actionType string, active bool, _ string) (*domaininteraction.Action, int, int, error) {
	r.setActionType = actionType
	r.setActionOn = active
	return r.action, r.likeCount, r.favoriteCount, nil
}

// CreateComment 返回传入的评论对象。
func (r *interactionHandlerRepository) CreateComment(_ context.Context, comment *domaininteraction.Comment) (*domaininteraction.Comment, int, error) {
	return comment, 1, nil
}

// FindCommentByUserAndIdempotencyKey 表示测试中没有已存在的幂等评论。
func (r *interactionHandlerRepository) FindCommentByUserAndIdempotencyKey(context.Context, int64, string) (*domaininteraction.Comment, int, error) {
	return nil, 0, domaininteraction.ErrCommentNotFound
}

// ListComments 返回空评论列表。
func (r *interactionHandlerRepository) ListComments(context.Context, int64, *domaininteraction.CommentCursor, int) ([]*domaininteraction.Comment, error) {
	return []*domaininteraction.Comment{}, nil
}

// DeleteComment 记录删除请求中的用户和角色，并返回预设评论。
func (r *interactionHandlerRepository) DeleteComment(_ context.Context, _ int64, userID int64, role string) (*domaininteraction.Comment, int, error) {
	r.deleteUserID = userID
	r.deleteRole = role
	return r.deleteComment, 0, nil
}

// GetUserProfile 返回最小用户资料，供不校验触发者的用例满足仓储接口。
func (r *interactionHandlerRepository) GetUserProfile(_ context.Context, userID int64) (*domaininteraction.UserProfile, error) {
	return &domaininteraction.UserProfile{ID: userID}, nil
}
// TestLikeReturnsAllCounts 验证点赞 handler 返回两个统计字段并传递启用状态。
func TestLikeReturnsAllCounts(t *testing.T) {
	repository := &interactionHandlerRepository{
		action:        &domaininteraction.Action{VideoID: 3, ActionType: domaininteraction.ActionTypeLike, Status: domaininteraction.ActionStatusActive},
		likeCount:     8,
		favoriteCount: 5,
	}
	handler := NewHandler(applicationinteraction.NewService(repository))
	recorder := httptest.NewRecorder()
	ctx := newInteractionTestContext(http.MethodPost, "/api/videos/3/like", `{}`, recorder)
	ctx.Params = gin.Params{{Key: "videoId", Value: "3"}}
	ctx.Set(middleware.ContextUserIDKey, int64(7))

	handler.Like(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("Like() status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), `"like_count":8`) || !strings.Contains(recorder.Body.String(), `"favorite_count":5`) {
		t.Fatalf("Like() response = %s, want both counts", recorder.Body.String())
	}
	if repository.setActionType != domaininteraction.ActionTypeLike || !repository.setActionOn {
		t.Fatalf("Like() SetAction = (%q, %t), want (%q, true)", repository.setActionType, repository.setActionOn, domaininteraction.ActionTypeLike)
	}
}

// TestUnlikePassesInactiveState 验证取消点赞 handler 向应用层传递取消状态。
func TestUnlikePassesInactiveState(t *testing.T) {
	repository := &interactionHandlerRepository{
		action: &domaininteraction.Action{VideoID: 3, ActionType: domaininteraction.ActionTypeLike, Status: domaininteraction.ActionStatusCanceled},
	}
	handler := NewHandler(applicationinteraction.NewService(repository))
	recorder := httptest.NewRecorder()
	ctx := newInteractionTestContext(http.MethodDelete, "/api/videos/3/like", "", recorder)
	ctx.Params = gin.Params{{Key: "videoId", Value: "3"}}
	ctx.Set(middleware.ContextUserIDKey, int64(7))

	handler.Unlike(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("Unlike() status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if repository.setActionType != domaininteraction.ActionTypeLike || repository.setActionOn {
		t.Fatalf("Unlike() SetAction = (%q, %t), want (%q, false)", repository.setActionType, repository.setActionOn, domaininteraction.ActionTypeLike)
	}
}

// TestDeleteCommentPassesRole 验证 handler 将认证上下文中的角色传递给应用服务。
func TestDeleteCommentPassesRole(t *testing.T) {
	repository := &interactionHandlerRepository{
		deleteComment: &domaininteraction.Comment{ID: 9, Status: domaininteraction.CommentStatusDeleted, CreatedAt: time.Now()},
	}
	handler := NewHandler(applicationinteraction.NewService(repository))
	recorder := httptest.NewRecorder()
	ctx := newInteractionTestContext(http.MethodDelete, "/api/comments/9", "", recorder)
	ctx.Params = gin.Params{{Key: "commentId", Value: "9"}}
	ctx.Set(middleware.ContextUserIDKey, int64(7))
	ctx.Set(middleware.ContextUserRoleKey, "admin")

	handler.DeleteComment(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("DeleteComment() status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if repository.deleteUserID != 7 || repository.deleteRole != "admin" {
		t.Fatalf("DeleteComment() arguments = (%d, %q), want (7, admin)", repository.deleteUserID, repository.deleteRole)
	}
}

// TestInteractionHandlerRejectsNilService 验证 handler 缺少应用服务时返回内部错误而不是 panic。
func TestInteractionHandlerRejectsNilService(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx := newInteractionTestContext(http.MethodPost, "/api/videos/3/like", `{}`, recorder)
	ctx.Params = gin.Params{{Key: "videoId", Value: "3"}}

	NewHandler(nil).Like(ctx)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("Like() status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

// newInteractionTestContext 创建互动 handler 使用的 Gin 测试上下文。
func newInteractionTestContext(method, target, body string, recorder *httptest.ResponseRecorder) *gin.Context {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = request
	return ctx
}

var _ domaininteraction.Repository = (*interactionHandlerRepository)(nil)
