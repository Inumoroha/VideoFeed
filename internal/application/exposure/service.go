package exposure

import (
	"context"
	"errors"
	domainexposure "github.com/inumoroha/video-feed/internal/domain/exposure"
)

var ErrSaveExposureFailed = errors.New("failed to save exposure")

// Service 组合观看行为仓储与可选消息发布器。
type Service struct {
	repo      domainexposure.Repository
	publisher ViewEventPublisher
}

// RecordViewEventResult 保存已落库的观看行为及可选曝光聚合。
type RecordViewEventResult struct {
	Event    *domainexposure.ViewEvent
	Exposure *domainexposure.Exposure
}

// ViewEventPublisher 定义观看行为落库后的画像消息发布能力。
type ViewEventPublisher interface {
	PublishViewEventRecorded(ctx context.Context, event *ViewEventRecordedEvent) error
}

type Option func(*Service)

// NewService 注入仓储并按顺序应用曝光服务选项。
func NewService(repo domainexposure.Repository, options ...Option) *Service {
	service := &Service{repo: repo}
	for _, option := range options {
		option(service)
	}
	return service
}

// WithViewEventPublisher 为曝光服务启用观看行为事件发布。
func WithViewEventPublisher(publisher ViewEventPublisher) Option {
	return func(s *Service) {
		s.publisher = publisher
	}
}

// RecordViewEvent 校验并保存观看行为和可选曝光聚合，提交后尝试发布消息；发布失败仍返回保存结果。
func (s *Service) RecordViewEvent(ctx context.Context, userID int64, videoID int64, scene string, requestID string, eventType string, watchMs int, completed bool) (*RecordViewEventResult, error) {
	event, err := domainexposure.NewViewEvent(userID, videoID, scene, requestID, eventType, watchMs, completed)
	if err != nil {
		return nil, err
	}

	savedEvent, exposure, err := s.repo.SaveViewEvent(ctx, event)
	if err != nil {
		if errors.Is(err, domainexposure.ErrVideoNotFound) {
			return nil, err
		}
		return nil, ErrSaveExposureFailed
	}
	s.publishViewEventRecorded(ctx, savedEvent, exposure)

	return &RecordViewEventResult{
		Event:    savedEvent,
		Exposure: exposure,
	}, nil
}

// publishViewEventRecorded 在发布器和事件有效时尝试发送消息，发布失败不影响落库结果。
func (s *Service) publishViewEventRecorded(ctx context.Context, event *domainexposure.ViewEvent, exposure *domainexposure.Exposure) {
	if s.publisher == nil {
		return
	}
	message := NewViewEventRecordedEvent(event, exposure)
	if message == nil {
		return
	}
	_ = s.publisher.PublishViewEventRecorded(ctx, message)
}
