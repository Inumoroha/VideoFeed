package interaction

import (
	"context"
	domaininteraction "github.com/inumoroha/video-feed/internal/domain/interaction"
)

// ActionEventConsumer 消费互动异步事件。
type ActionEventConsumer interface {
	ConsumeActionChanged(context.Context, func(context.Context, *ActionChangedEvent) error) error
}

// ActionWorker 将异步事件写入互动仓储。
type ActionWorker struct {
	repo     domaininteraction.Repository
	consumer ActionEventConsumer
}

// NewActionWorker 创建互动事件 Worker。
func NewActionWorker(repo domaininteraction.Repository, consumer ActionEventConsumer) *ActionWorker {
	return &ActionWorker{repo: repo, consumer: consumer}
}

// Start 启动互动事件消费。
func (w *ActionWorker) Start(ctx context.Context) error {
	if w == nil || w.consumer == nil {
		return nil
	}
	return w.consumer.ConsumeActionChanged(ctx, w.HandleActionChanged)
}

// HandleActionChanged 处理单条互动事件。
func (w *ActionWorker) HandleActionChanged(ctx context.Context, e *ActionChangedEvent) error {
	if e == nil {
		return nil
	}
	_, _, _, err := w.repo.SetAction(ctx, e.UserID, e.VideoID, e.ActionType, e.Active, e.IdempotencyKey)
	return err
}
