package embedding_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	applicationembedding "github.com/inumoroha/video-feed/internal/application/embedding"
	applicationvideo "github.com/inumoroha/video-feed/internal/application/video"
	domainembedding "github.com/inumoroha/video-feed/internal/domain/embedding"
)

// memoryVideoEmbeddingRepo 保存顺序执行的向量消费者测试所需内存记录。
type memoryVideoEmbeddingRepo struct {
	items map[string]*domainembedding.VideoEmbedding
}

// newMemoryVideoEmbeddingRepo 初始化测试仓储的视频模型记录映射。
func newMemoryVideoEmbeddingRepo() *memoryVideoEmbeddingRepo {
	return &memoryVideoEmbeddingRepo{items: map[string]*domainembedding.VideoEmbedding{}}
}

// SaveVideoEmbedding 按视频和模型覆盖存储复制的向量对象。
func (r *memoryVideoEmbeddingRepo) SaveVideoEmbedding(ctx context.Context, embedding *domainembedding.VideoEmbedding) error {
	r.items[videoEmbeddingKey(embedding.VideoID, embedding.Model)] = cloneVideoEmbedding(embedding)
	return nil
}

// FindVideoEmbedding 按视频和模型返回记录副本，缺失时返回领域错误。
func (r *memoryVideoEmbeddingRepo) FindVideoEmbedding(ctx context.Context, videoID int64, model string) (*domainembedding.VideoEmbedding, error) {
	item, exists := r.items[videoEmbeddingKey(videoID, model)]
	if !exists {
		return nil, domainembedding.ErrVideoEmbeddingNotFound
	}
	return cloneVideoEmbedding(item), nil
}

// TestVideoEmbeddingWorkerWritesEmbedding 验证发布事件生成并保存向量、模型元数据和文本哈希。
func TestVideoEmbeddingWorkerWritesEmbedding(t *testing.T) {
	repo := newMemoryVideoEmbeddingRepo()
	service := applicationembedding.NewService(repo, domainembedding.NewHashNgramVectorizer())
	worker := applicationembedding.NewVideoEmbeddingWorker(service, nil)

	event := &applicationvideo.PublishedEvent{
		VideoID:     1001,
		AuthorID:    42,
		Title:       "篮球训练",
		Description: "投篮技巧",
		PublishedAt: time.Now(),
	}
	if err := worker.HandleVideoPublished(context.Background(), event); err != nil {
		t.Fatalf("handle video published: %v", err)
	}

	embedding, err := repo.FindVideoEmbedding(context.Background(), 1001, domainembedding.HashNgramModel)
	if err != nil {
		t.Fatalf("find video embedding: %v", err)
	}
	if embedding.VideoID != 1001 || embedding.Model != domainembedding.HashNgramModel || embedding.Dimension != domainembedding.HashNgramDimension {
		t.Fatalf("unexpected embedding metadata: %+v", embedding)
	}
	if embedding.TextHash != domainembedding.TextHash("篮球训练\n投篮技巧") {
		t.Fatalf("unexpected text hash: %s", embedding.TextHash)
	}
	if embedding.EmbeddingJSON == "" || len(embedding.Embedding) != domainembedding.HashNgramDimension {
		t.Fatalf("unexpected embedding payload: %+v", embedding)
	}
}

// TestVideoEmbeddingWorkerUpdatesByVideoAndModel 验证相同视频模型的后续事件覆盖旧记录。
func TestVideoEmbeddingWorkerUpdatesByVideoAndModel(t *testing.T) {
	repo := newMemoryVideoEmbeddingRepo()
	service := applicationembedding.NewService(repo, domainembedding.NewHashNgramVectorizer())
	worker := applicationembedding.NewVideoEmbeddingWorker(service, nil)

	first := &applicationvideo.PublishedEvent{
		VideoID:     1002,
		AuthorID:    42,
		Title:       "篮球训练",
		Description: "投篮技巧",
		PublishedAt: time.Now(),
	}
	second := &applicationvideo.PublishedEvent{
		VideoID:     1002,
		AuthorID:    42,
		Title:       "美食探店",
		Description: "火锅推荐",
		PublishedAt: time.Now(),
	}

	if err := worker.HandleVideoPublished(context.Background(), first); err != nil {
		t.Fatalf("handle first video published: %v", err)
	}
	firstEmbedding, err := repo.FindVideoEmbedding(context.Background(), 1002, domainembedding.HashNgramModel)
	if err != nil {
		t.Fatalf("find first embedding: %v", err)
	}
	if err := worker.HandleVideoPublished(context.Background(), second); err != nil {
		t.Fatalf("handle second video published: %v", err)
	}
	secondEmbedding, err := repo.FindVideoEmbedding(context.Background(), 1002, domainembedding.HashNgramModel)
	if err != nil {
		t.Fatalf("find second embedding: %v", err)
	}

	if len(repo.items) != 1 {
		t.Fatalf("expected one video+model embedding, got %d", len(repo.items))
	}
	if firstEmbedding.TextHash == secondEmbedding.TextHash {
		t.Fatalf("embedding was not updated")
	}
}

// cloneVideoEmbedding 复制测试领域对象及向量切片。
func cloneVideoEmbedding(embedding *domainembedding.VideoEmbedding) *domainembedding.VideoEmbedding {
	cloned := *embedding
	cloned.Embedding = append([]float64(nil), embedding.Embedding...)
	return &cloned
}

// videoEmbeddingKey 将视频编号和模型拼成内存测试索引键。
func videoEmbeddingKey(videoID int64, model string) string {
	return fmt.Sprintf("%d:%s", videoID, model)
}
