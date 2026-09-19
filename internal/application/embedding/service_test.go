package embedding

import (
	"context"
	"errors"
	"math"
	"testing"

	applicationvideo "github.com/inumoroha/video-feed/internal/application/video"
	domainembedding "github.com/inumoroha/video-feed/internal/domain/embedding"
)

// recordingRepository 记录服务保存的对象与上下文，并模拟存储失败。
type recordingRepository struct {
	domainembedding.Repository
	saved *domainembedding.VideoEmbedding
	ctx   context.Context
	calls int
	err   error
}

// SaveVideoEmbedding 保存测试调用参数并返回预设错误。
func (r *recordingRepository) SaveVideoEmbedding(ctx context.Context, value *domainembedding.VideoEmbedding) error {
	r.saved = value
	r.ctx = ctx
	r.calls++
	return r.err
}

// invalidVectorizer 用非有限数触发真实 JSON 序列化错误。
type invalidVectorizer struct{}

// Model 返回测试模型名称。
func (invalidVectorizer) Model() string { return "invalid" }

// Dimension 返回测试向量维度。
func (invalidVectorizer) Dimension() int { return 1 }

// Vectorize 返回无法序列化为 JSON 的非有限向量。
func (invalidVectorizer) Vectorize(string) []float64 { return []float64{math.NaN()} }

// recordingConsumer 记录订阅上下文和消息处理器，并可返回订阅错误。
type recordingConsumer struct {
	ctx     context.Context
	handler func(context.Context, *applicationvideo.PublishedEvent) error
	err     error
}

// ConsumeVideoPublishedForEmbedding 保存订阅参数并返回预设结果。
func (c *recordingConsumer) ConsumeVideoPublishedForEmbedding(ctx context.Context, handler func(context.Context, *applicationvideo.PublishedEvent) error) error {
	c.ctx = ctx
	c.handler = handler
	return c.err
}

// TestServiceFailureAndReplay 验证无效事件忽略、默认向量器、上下文传递、重复保存和错误映射。
func TestServiceFailureAndReplay(t *testing.T) {
	repo := &recordingRepository{}
	service := NewService(repo, nil)
	for _, event := range []*applicationvideo.PublishedEvent{nil, {VideoID: 0}, {VideoID: -1}} {
		result, err := service.GenerateForPublishedVideo(context.Background(), event)
		if err != nil || result.Embedding != nil || result.CreatedOrUpdated || repo.calls != 0 {
			t.Fatalf("result=%+v err=%v", result, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	event := &applicationvideo.PublishedEvent{VideoID: 1, Title: " title ", Description: " desc "}
	for i := 0; i < 2; i++ {
		result, err := service.GenerateForPublishedVideo(ctx, event)
		if err != nil || !result.CreatedOrUpdated || result.Embedding != repo.saved || repo.ctx != ctx || result.Embedding.Dimension != 128 || result.Embedding.TextHash != domainembedding.TextHash("title\ndesc") {
			t.Fatalf("result=%+v err=%v", result, err)
		}
	}
	if repo.calls != 2 {
		t.Fatal("同内容重放被错误跳过")
	}
	repo.err = errors.New("private database detail")
	if result, err := service.GenerateForPublishedVideo(ctx, event); err != ErrSaveVideoEmbeddingFailed || result != nil {
		t.Fatal(result, err)
	}
	bad := NewService(repo, invalidVectorizer{})
	if result, err := bad.GenerateForPublishedVideo(ctx, event); err != ErrMarshalEmbeddingFailed || result != nil || repo.calls != 3 {
		t.Fatal(result, err)
	}
}

// TestWorkerLifecycle 验证空依赖忽略、订阅错误传播以及保存失败可重试。
func TestWorkerLifecycle(t *testing.T) {
	var empty *VideoEmbeddingWorker
	ctx := context.Background()
	for _, worker := range []*VideoEmbeddingWorker{empty, {}} {
		if err := worker.Start(ctx); err != nil {
			t.Fatal(err)
		}
		if err := worker.HandleVideoPublished(ctx, &applicationvideo.PublishedEvent{VideoID: 1}); err != nil {
			t.Fatal(err)
		}
	}
	failure := errors.New("consume failed")
	consumer := &recordingConsumer{err: failure}
	repo := &recordingRepository{err: errors.New("retry")}
	worker := NewVideoEmbeddingWorker(NewService(repo, nil), consumer)
	if err := worker.Start(ctx); !errors.Is(err, failure) || consumer.ctx != ctx || consumer.handler == nil {
		t.Fatal(err)
	}
	if err := consumer.handler(ctx, nil); err != nil || repo.calls != 0 {
		t.Fatal(err)
	}
	event := &applicationvideo.PublishedEvent{VideoID: 1, Title: "test"}
	if err := consumer.handler(ctx, event); err != ErrSaveVideoEmbeddingFailed {
		t.Fatal(err)
	}
	repo.err = nil
	if err := consumer.handler(ctx, event); err != nil || repo.calls != 2 {
		t.Fatal(err)
	}
}
