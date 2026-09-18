package video

import (
	"context"
	"errors"
	"strings"

	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"
)

const MaxListLimit = 100

// Service 保存视频仓储和可选发布事件发送器。
type Service struct {
	repo      domainvideo.Repository
	publisher PublishedEventPublisher
}

// NewService 创建使用指定视频仓储和可选发布器的应用服务。
func NewService(repo domainvideo.Repository, options ...Option) *Service {
	service := &Service{repo: repo}
	for _, option := range options {
		option(service)
	}
	return service
}

// CreatePublished 校验发布参数，按幂等键复用已有视频或创建新视频。
func (s *Service) CreatePublished(ctx context.Context, authorID int64, title, description string, mediaURL, coverURL, idempotencyKey string) (*CreateResult, error) {
	if !s.ready() {
		return nil, ErrVideoServiceNotInitialized
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	video, err := domainvideo.NewPublished(authorID, title, description, mediaURL, coverURL, idempotencyKey)
	if err != nil {
		return nil, err
	}

	if idempotencyKey != "" {
		existing, err := s.repo.FindByAuthorAndIdempotencyKey(ctx, authorID, idempotencyKey)
		if err == nil {
			if existing == nil {
				return nil, ErrLoadVideoFailed
			}
			return &CreateResult{Video: existing, Created: false}, nil
		}
		if !errors.Is(err, domainvideo.ErrVideoNotFound) {
			return nil, ErrLoadVideoFailed
		}
	}

	if err := s.repo.Save(ctx, video); err != nil {
		if idempotencyKey != "" && errors.Is(err, domainvideo.ErrDuplicateIdempotencyKey) {
			existing, loadErr := s.repo.FindByAuthorAndIdempotencyKey(ctx, authorID, idempotencyKey)
			if loadErr == nil {
				if existing == nil {
					return nil, ErrLoadVideoFailed
				}
				return &CreateResult{Video: existing, Created: false}, nil
			}
			return nil, ErrLoadVideoFailed
		}
		return nil, ErrSaveVideoFailed
	}

	s.publishCreatedVideo(ctx, video)
	return &CreateResult{Video: video, Created: true}, nil
}

// Get 查询指定的已发布视频，并将仓储错误转换为应用层错误。
func (s *Service) Get(ctx context.Context, videoID int64) (*domainvideo.Video, error) {
	if !s.ready() {
		return nil, ErrVideoServiceNotInitialized
	}
	if videoID <= 0 {
		return nil, domainvideo.ErrInvalidVideoID
	}

	video, err := s.repo.FindByID(ctx, videoID)
	if err != nil {
		if errors.Is(err, domainvideo.ErrVideoNotFound) {
			return nil, domainvideo.ErrVideoNotFound
		}
		return nil, ErrLoadVideoFailed
	}
	if video == nil {
		return nil, ErrLoadVideoFailed
	}
	return video, nil
}

// ListByAuthor 分页查询作者的已发布视频，并限制单次查询数量。
func (s *Service) ListByAuthor(ctx context.Context, authorID int64, limit, offset int) ([]*domainvideo.Video, error) {
	if !s.ready() {
		return nil, ErrVideoServiceNotInitialized
	}
	if authorID <= 0 {
		return nil, domainvideo.ErrInvalidAuthorID
	}
	if limit <= 0 {
		return nil, domainvideo.ErrInvalidLimit
	}
	if offset < 0 {
		return nil, domainvideo.ErrInvalidOffset
	}
	if limit > MaxListLimit {
		limit = MaxListLimit
	}

	videos, err := s.repo.ListByAuthor(ctx, authorID, limit, offset)
	if err != nil {
		return nil, ErrLoadVideoFailed
	}
	return videos, nil
}

// Delete 校验作者权限后将指定视频标记为已删除。
func (s *Service) Delete(ctx context.Context, authorID, videoID int64) error {
	if !s.ready() {
		return ErrVideoServiceNotInitialized
	}
	if authorID <= 0 {
		return domainvideo.ErrInvalidAuthorID
	}
	if videoID <= 0 {
		return domainvideo.ErrInvalidVideoID
	}

	video, err := s.repo.FindByIDAnyStatus(ctx, videoID)
	if err != nil {
		if errors.Is(err, domainvideo.ErrVideoNotFound) {
			return domainvideo.ErrVideoNotFound
		}
		return ErrLoadVideoFailed
	}
	if video == nil {
		return ErrLoadVideoFailed
	}
	alreadyDeleted := video.Status == domainvideo.StatusDeleted
	if err := video.DeleteBy(authorID); err != nil {
		return err
	}
	if alreadyDeleted {
		return nil
	}
	if err := s.repo.UpdateStatus(ctx, video); err != nil {
		if errors.Is(err, domainvideo.ErrVideoNotFound) {
			return domainvideo.ErrVideoNotFound
		}
		return ErrUpdateVideoFailed
	}
	return nil
}

// ready 判断服务及其视频仓储依赖是否已初始化。
func (s *Service) ready() bool {
	return s != nil && s.repo != nil
}

// PublishedEventPublisher 定义视频发布事件发送能力。
type PublishedEventPublisher interface {
	PublishVideoPublished(context.Context, *PublishedEvent) error
}

// Option 配置视频服务的可选依赖。
type Option func(*Service)

// WithPublishedEventPublisher 注入视频创建后的发布事件发送器。
func WithPublishedEventPublisher(publisher PublishedEventPublisher) Option {
	return func(s *Service) {
		s.publisher = publisher
	}
}

// publishCreatedVideo 发送新建视频事件；发送失败不回滚已完成的视频事务。
func (s *Service) publishCreatedVideo(ctx context.Context, video *domainvideo.Video) {
	if s.publisher == nil {
		return
	}
	event := NewPublishedEvent(video)
	if event == nil {
		return
	}
	_ = s.publisher.PublishVideoPublished(ctx, event)
}
