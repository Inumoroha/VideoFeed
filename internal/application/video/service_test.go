package video

import (
	"context"
	"errors"
	"testing"

	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"
)

type serviceTestRepository struct {
	findByKeyCalls int
	findByKeyVideo *domainvideo.Video
	findByKeyErr   error
	findByIDVideo  *domainvideo.Video
	findByIDErr    error
	findAnyVideo   *domainvideo.Video
	findAnyErr     error
	listVideos     []*domainvideo.Video
	listErr        error
	saveErr        error
	savedVideo     *domainvideo.Video
	updateErr      error
}

// Save 记录测试视频并模拟仓储保存结果。
func (r *serviceTestRepository) Save(_ context.Context, video *domainvideo.Video) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.savedVideo = video
	if video.ID == 0 {
		video.ID = 1
	}
	return nil
}

// FindByID 返回测试用的已发布视频或预设错误。
func (r *serviceTestRepository) FindByID(_ context.Context, _ int64) (*domainvideo.Video, error) {
	if r.findByIDErr != nil {
		return nil, r.findByIDErr
	}
	return r.findByIDVideo, nil
}

// FindByIDAnyStatus 返回测试用的任意状态视频或预设错误。
func (r *serviceTestRepository) FindByIDAnyStatus(_ context.Context, _ int64) (*domainvideo.Video, error) {
	if r.findAnyErr != nil {
		return nil, r.findAnyErr
	}
	return r.findAnyVideo, nil
}

// FindByAuthorAndIdempotencyKey 记录幂等查询次数并返回预设结果。
func (r *serviceTestRepository) FindByAuthorAndIdempotencyKey(_ context.Context, _ int64, _ string) (*domainvideo.Video, error) {
	r.findByKeyCalls++
	if r.findByKeyErr != nil {
		return nil, r.findByKeyErr
	}
	return r.findByKeyVideo, nil
}

// ListByAuthor 返回测试用的视频列表或预设错误。
func (r *serviceTestRepository) ListByAuthor(_ context.Context, _ int64, _, _ int) ([]*domainvideo.Video, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return r.listVideos, nil
}

// UpdateStatus 返回预设的视频状态更新错误。
func (r *serviceTestRepository) UpdateStatus(_ context.Context, _ *domainvideo.Video) error {
	return r.updateErr
}

// TestCreatePublishedValidatesBeforeIdempotencyLookup 验证发布参数在幂等查询前完成校验。
func TestCreatePublishedValidatesBeforeIdempotencyLookup(t *testing.T) {
	repository := &serviceTestRepository{}
	service := NewService(repository)

	_, err := service.CreatePublished(context.Background(), 0, "title", "media", "cover", "key")
	if !errors.Is(err, domainvideo.ErrInvalidAuthorID) {
		t.Fatalf("CreatePublished() error = %v, want %v", err, domainvideo.ErrInvalidAuthorID)
	}
	if repository.findByKeyCalls != 0 {
		t.Fatalf("CreatePublished() idempotency lookups = %d, want 0", repository.findByKeyCalls)
	}
}

// TestCreatePublishedCreatesAndReusesVideo 验证无幂等键时创建视频并在已有幂等记录时复用视频。
func TestCreatePublishedCreatesAndReusesVideo(t *testing.T) {
	repository := &serviceTestRepository{}
	service := NewService(repository)

	created, err := service.CreatePublished(context.Background(), 1, "title", "media", "cover", "")
	if err != nil {
		t.Fatalf("CreatePublished() error = %v", err)
	}
	if created == nil || !created.Created || created.Video == nil || repository.savedVideo == nil {
		t.Fatalf("CreatePublished() result = %+v, want created video", created)
	}

	existing := &domainvideo.Video{ID: 9, AuthorID: 1, Status: domainvideo.StatusPublished}
	repository.findByKeyVideo = existing
	result, err := service.CreatePublished(context.Background(), 1, "title", "media", "cover", "key")
	if err != nil {
		t.Fatalf("CreatePublished() existing error = %v", err)
	}
	if result == nil || result.Created || result.Video != existing {
		t.Fatalf("CreatePublished() existing result = %+v, want reused video", result)
	}
}

// TestServiceRejectsMissingDependency 验证服务缺少仓储依赖时返回错误而不触发 panic。
func TestServiceRejectsMissingDependency(t *testing.T) {
	service := NewService(nil)
	if _, err := service.Get(context.Background(), 1); !errors.Is(err, ErrVideoServiceNotInitialized) {
		t.Fatalf("Get() error = %v, want %v", err, ErrVideoServiceNotInitialized)
	}
	if err := service.Delete(context.Background(), 1, 1); !errors.Is(err, ErrVideoServiceNotInitialized) {
		t.Fatalf("Delete() error = %v, want %v", err, ErrVideoServiceNotInitialized)
	}
}

// TestServiceRejectsNilRepositoryResults 验证仓储返回空对象时服务不会返回空成功结果或 panic。
func TestServiceRejectsNilRepositoryResults(t *testing.T) {
	service := NewService(&serviceTestRepository{})

	if _, err := service.Get(context.Background(), 1); !errors.Is(err, ErrLoadVideoFailed) {
		t.Fatalf("Get() error = %v, want %v", err, ErrLoadVideoFailed)
	}
	if _, err := service.CreatePublished(context.Background(), 1, "title", "media", "cover", "key"); !errors.Is(err, ErrLoadVideoFailed) {
		t.Fatalf("CreatePublished() error = %v, want %v", err, ErrLoadVideoFailed)
	}
	if err := service.Delete(context.Background(), 1, 1); !errors.Is(err, ErrLoadVideoFailed) {
		t.Fatalf("Delete() error = %v, want %v", err, ErrLoadVideoFailed)
	}
}

var _ domainvideo.Repository = (*serviceTestRepository)(nil)
