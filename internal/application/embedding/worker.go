package embedding

import (
	"context"
	applicationvideo "github.com/inumoroha/video-feed/internal/application/video"
)

// PublishedEventConsumer 消费视频发布事件。
type PublishedEventConsumer interface {
	ConsumeVideoPublishedForEmbedding(ctx context.Context, handler func(context.Context, *applicationvideo.PublishedEvent) error) error
}

// VideoEmbeddingWorker 将独立视频发布队列的消息交给向量生成服务。
type VideoEmbeddingWorker struct {
	service  *Service
	consumer PublishedEventConsumer
}

// NewVideoEmbeddingWorker 注入向量服务和视频发布消息消费者。
func NewVideoEmbeddingWorker(service *Service, consumer PublishedEventConsumer) *VideoEmbeddingWorker {
	return &VideoEmbeddingWorker{
		service:  service,
		consumer: consumer,
	}
}

// Start 订阅向量专用视频发布队列；空接收者或消费者不启动，订阅错误向调用者返回。
func (w *VideoEmbeddingWorker) Start(ctx context.Context) error {
	if w == nil || w.consumer == nil {
		return nil
	}
	return w.consumer.ConsumeVideoPublishedForEmbedding(ctx, w.HandleVideoPublished)
}

// HandleVideoPublished 处理视频发布消息并返回生成或存储错误供消息层重试；缺失服务或事件时忽略。
func (w *VideoEmbeddingWorker) HandleVideoPublished(ctx context.Context, event *applicationvideo.PublishedEvent) error {
	if w == nil || w.service == nil || event == nil {
		return nil
	}
	_, err := w.service.GenerateForPublishedVideo(ctx, event)
	return err
}
