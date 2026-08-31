package feed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	applicationfeed "github.com/inumoroha/video-feed/internal/application/feed"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	"github.com/inumoroha/video-feed/internal/middleware"
)

// handlerTestRepository 保存 handler 测试中的事件并模拟信息流仓储行为。
type handlerTestRepository struct {
	savedEvent *domainfeed.ViewEvent
}

// ListTimeFeed 返回空的信息流结果，满足 handler 测试所需的仓储接口。
func (r *handlerTestRepository) ListTimeFeed(_ context.Context, _ *domainfeed.TimeCursor, _ int) ([]*domainfeed.FeedItem, error) {
	return []*domainfeed.FeedItem{}, nil
}

// SaveViewEvent 记录观看事件并模拟数据库生成 ID 和创建时间。
func (r *handlerTestRepository) SaveViewEvent(_ context.Context, event *domainfeed.ViewEvent) error {
	event.ID = 1
	event.CreatedAt = time.Date(2026, time.August, 31, 12, 0, 0, 0, time.UTC)
	cloned := *event
	if event.UserID != nil {
		userID := *event.UserID
		cloned.UserID = &userID
	}
	r.savedEvent = &cloned
	return nil
}

// FindViewEventByIdempotencyKey 模拟未找到幂等键对应的观看事件。
func (r *handlerTestRepository) FindViewEventByIdempotencyKey(_ context.Context, _ string) (*domainfeed.ViewEvent, error) {
	return nil, domainfeed.ErrViewEventNotFound
}

// TestTimeWithNilServiceReturnsInternalServerError 验证信息流服务未初始化时不会触发 panic。
func TestTimeWithNilServiceReturnsInternalServerError(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx := newFeedTestContext(http.MethodGet, "/api/feed/time", "", recorder)

	NewHandler(nil).Time(ctx)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("Time() status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

// TestReportViewEventRejectsMissingActor 验证访客和登录用户均缺失时返回客户端错误。
func TestReportViewEventRejectsMissingActor(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx := newFeedTestContext(http.MethodPost, "/api/feed/view-events", `{"video_id":1,"event_type":"VIEW"}`, recorder)

	NewHandler(applicationfeed.NewService(&handlerTestRepository{})).ReportViewEvent(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("ReportViewEvent() status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

// TestReportViewEventUsesAuthenticatedUser 验证认证上下文中的用户 ID 会写入观看事件。
func TestReportViewEventUsesAuthenticatedUser(t *testing.T) {
	repository := &handlerTestRepository{}
	recorder := httptest.NewRecorder()
	ctx := newFeedTestContext(http.MethodPost, "/api/feed/view-events", `{"video_id":1,"event_type":"VIEW"}`, recorder)
	ctx.Set(middleware.ContextUserIDKey, int64(7))

	NewHandler(applicationfeed.NewService(repository)).ReportViewEvent(ctx)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("ReportViewEvent() status = %d, want %d; body = %s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	if repository.savedEvent == nil || repository.savedEvent.UserID == nil || *repository.savedEvent.UserID != 7 {
		t.Fatalf("saved event = %+v, want authenticated user 7", repository.savedEvent)
	}
}

// newFeedTestContext 创建带 JSON 请求体的 Gin 测试上下文。
func newFeedTestContext(method, target, body string, recorder *httptest.ResponseRecorder) *gin.Context {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = request
	return ctx
}

var _ domainfeed.Repository = (*handlerTestRepository)(nil)
