package relation

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	applicationrelation "github.com/inumoroha/video-feed/internal/application/relation"
	domainrelation "github.com/inumoroha/video-feed/internal/domain/relation"
	"github.com/inumoroha/video-feed/internal/middleware"
)

// relationHandlerRepository 是关系处理器测试使用的内存仓储替身。
type relationHandlerRepository struct{}

// SetFollow 返回预设的关注结果。
func (relationHandlerRepository) SetFollow(_ context.Context, userID, targetUserID int64, active bool, key string) (*domainrelation.Follow, *domainrelation.RelationStat, *domainrelation.RelationStat, error) {
	return domainrelation.RestoreFollow(1, userID, targetUserID, map[bool]int{true: 1, false: 2}[active], key, time.Time{}, time.Time{}), domainrelation.RestoreRelationStat(userID, 3, 1, time.Time{}, time.Time{}), domainrelation.RestoreRelationStat(targetUserID, 2, 4, time.Time{}, time.Time{}), nil
}

// ListFollowing 返回空关注列表。
func (relationHandlerRepository) ListFollowing(context.Context, int64, *domainrelation.ListCursor, int) ([]*domainrelation.UserItem, error) {
	return []*domainrelation.UserItem{}, nil
}

// ListFollowers 返回空粉丝列表。
func (relationHandlerRepository) ListFollowers(context.Context, int64, *domainrelation.ListCursor, int) ([]*domainrelation.UserItem, error) {
	return []*domainrelation.UserItem{}, nil
}

// GetUserProfile 返回空资料，供不校验触发者的用例满足仓储接口。
func (relationHandlerRepository) GetUserProfile(context.Context, int64) (*domainrelation.UserProfile, error) {
	return domainrelation.RestoreUserProfile(0, "", "", ""), nil
}
// TestHandlerRejectsNilService 验证空服务依赖返回内部错误。
func TestHandlerRejectsNilService(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx := relationTestContext(http.MethodGet, "/api/relations/following", recorder)
	NewHandler(nil).ListFollowing(ctx)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("ListFollowing() status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

// TestFollowPassesAuthAndIdempotencyKey 验证关注接口读取认证用户和幂等键并返回关系结果。
func TestFollowPassesAuthAndIdempotencyKey(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx := relationTestContext(http.MethodPut, "/api/relations/3", recorder)
	ctx.Params = gin.Params{{Key: "targetUserId", Value: "3"}}
	ctx.Set(middleware.ContextUserIDKey, int64(7))
	ctx.Request.Header.Set("Idempotency-Key", "request-1")
	NewHandler(applicationrelation.NewService(relationHandlerRepository{})).Follow(ctx)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"following":true`) || !strings.Contains(recorder.Body.String(), `"following_count":3`) {
		t.Fatalf("Follow() response = (%d) %s", recorder.Code, recorder.Body.String())
	}
}

// TestListFollowingRejectsMissingAuth 验证关注列表缺少认证上下文时返回未认证。
func TestListFollowingRejectsMissingAuth(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx := relationTestContext(http.MethodGet, "/api/relations/following", recorder)
	NewHandler(applicationrelation.NewService(relationHandlerRepository{})).ListFollowing(ctx)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("ListFollowing() status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

// relationTestContext 创建关系处理器测试使用的 Gin 上下文。
func relationTestContext(method, target string, recorder *httptest.ResponseRecorder) *gin.Context {
	request := httptest.NewRequest(method, target, nil)
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = request
	return ctx
}

var _ domainrelation.Repository = relationHandlerRepository{}
