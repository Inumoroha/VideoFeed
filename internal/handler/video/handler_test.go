package video

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	applicationvideo "github.com/inumoroha/video-feed/internal/application/video"
	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"
	"github.com/inumoroha/video-feed/internal/middleware"
)

type handlerTestRepository struct {
	savedVideo  *domainvideo.Video
	findVideo   *domainvideo.Video
	findErr     error
	listVideos  []*domainvideo.Video
	listLimit   int
	listOffset  int
	updateVideo *domainvideo.Video
}

// Save 记录测试视频并模拟保存成功。
func (r *handlerTestRepository) Save(_ context.Context, video *domainvideo.Video) error {
	r.savedVideo = video
	video.ID = 1
	return nil
}

// FindByID 返回测试视频或未找到错误。
func (r *handlerTestRepository) FindByID(_ context.Context, _ int64) (*domainvideo.Video, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	return r.findVideo, nil
}

// FindByIDAnyStatus 返回用于删除测试的视频或预设错误。
func (r *handlerTestRepository) FindByIDAnyStatus(_ context.Context, _ int64) (*domainvideo.Video, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	return r.findVideo, nil
}

// FindByAuthorAndIdempotencyKey 返回测试幂等记录或未找到错误。
func (r *handlerTestRepository) FindByAuthorAndIdempotencyKey(_ context.Context, _ int64, _ string) (*domainvideo.Video, error) {
	return nil, domainvideo.ErrVideoNotFound
}

// ListByAuthor 记录分页参数并返回测试视频列表。
func (r *handlerTestRepository) ListByAuthor(_ context.Context, _ int64, limit, offset int) ([]*domainvideo.Video, error) {
	r.listLimit = limit
	r.listOffset = offset
	return r.listVideos, nil
}

// UpdateStatus 记录待更新的视频并模拟状态更新。
func (r *handlerTestRepository) UpdateStatus(_ context.Context, video *domainvideo.Video) error {
	r.updateVideo = video
	return nil
}

// TestCreateRequiresAuthentication 验证创建视频缺少认证用户时返回 401。
func TestCreateRequiresAuthentication(t *testing.T) {
	handler := NewHandler(applicationvideo.NewService(&handlerTestRepository{}))
	recorder := httptest.NewRecorder()
	ctx := newVideoTestContext(http.MethodPost, "/api/videos", `{"title":"title","media_url":"media","cover_url":"cover"}`, recorder)

	handler.Create(ctx)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("Create() status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

// TestCreateReturnsCreatedVideo 验证创建成功后返回 201 和视频响应。
func TestCreateReturnsCreatedVideo(t *testing.T) {
	repository := &handlerTestRepository{}
	handler := NewHandler(applicationvideo.NewService(repository))
	recorder := httptest.NewRecorder()
	ctx := newVideoTestContext(http.MethodPost, "/api/videos", `{"title":"title","media_url":"media","cover_url":"cover"}`, recorder)
	ctx.Set(middleware.ContextUserIDKey, int64(7))

	handler.Create(ctx)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("Create() status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	if repository.savedVideo == nil || repository.savedVideo.AuthorID != 7 {
		t.Fatalf("Create() saved video = %+v, want author 7", repository.savedVideo)
	}
	if !strings.Contains(recorder.Body.String(), `"id":1`) {
		t.Fatalf("Create() response = %s, want video id", recorder.Body.String())
	}
}

// TestCreateWithNilServiceReturnsInternalServerError 验证处理器缺少服务时返回 500 而不 panic。
func TestCreateWithNilServiceReturnsInternalServerError(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx := newVideoTestContext(http.MethodPost, "/api/videos", `{}`, recorder)

	NewHandler(nil).Create(ctx)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("Create() status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

// TestListMineCapsLimitInResponse 验证超过上限的分页请求会同步限制查询参数和响应元数据。
func TestListMineCapsLimitInResponse(t *testing.T) {
	repository := &handlerTestRepository{}
	handler := NewHandler(applicationvideo.NewService(repository))
	recorder := httptest.NewRecorder()
	ctx := newVideoTestContext(http.MethodGet, "/api/videos/mine?limit=1000&offset=2", "", recorder)
	ctx.Set(middleware.ContextUserIDKey, int64(7))

	handler.ListMine(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("ListMine() status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if repository.listLimit != applicationvideo.MaxListLimit || repository.listOffset != 2 {
		t.Fatalf("ListMine() pagination = (%d, %d), want (%d, 2)", repository.listLimit, repository.listOffset, applicationvideo.MaxListLimit)
	}
	if !strings.Contains(recorder.Body.String(), `"limit":100`) {
		t.Fatalf("ListMine() response = %s, want capped limit", recorder.Body.String())
	}
}

// TestDeleteMapsPermissionDenied 验证删除非本人视频时返回 403。
func TestDeleteMapsPermissionDenied(t *testing.T) {
	repository := &handlerTestRepository{findVideo: &domainvideo.Video{ID: 1, AuthorID: 8, Status: domainvideo.StatusPublished}}
	handler := NewHandler(applicationvideo.NewService(repository))
	recorder := httptest.NewRecorder()
	ctx := newVideoTestContext(http.MethodDelete, "/api/videos/1", "", recorder)
	ctx.Params = gin.Params{{Key: "videoId", Value: "1"}}
	ctx.Set(middleware.ContextUserIDKey, int64(7))

	handler.Delete(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("Delete() status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

// newVideoTestContext 创建带请求体的 Gin 测试上下文。
func newVideoTestContext(method, target, body string, recorder *httptest.ResponseRecorder) *gin.Context {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = request
	return ctx
}

var _ domainvideo.Repository = (*handlerTestRepository)(nil)
