package embedding

import (
	"context"
	"encoding/json"
	"errors"
	applicationvideo "github.com/inumoroha/video-feed/internal/application/video"
	domainembedding "github.com/inumoroha/video-feed/internal/domain/embedding"
)

var ErrSaveVideoEmbeddingFailed = errors.New("failed to save video embedding")
var ErrMarshalEmbeddingFailed = errors.New("failed to marshal embedding")

// Service 组合视频向量仓储和可替换的文本向量器。
type Service struct {
	repo       domainembedding.Repository
	vectorizer domainembedding.Vectorizer
}

// GenerateVideoEmbeddingResult 保存生成结果，并标记本次是否成功执行存储。
type GenerateVideoEmbeddingResult struct {
	Embedding        *domainembedding.VideoEmbedding
	CreatedOrUpdated bool
}

// NewService 注入仓储和向量器，未指定向量器时使用默认本地哈希实现。
func NewService(repo domainembedding.Repository, vectorizer domainembedding.Vectorizer) *Service {
	if vectorizer == nil {
		vectorizer = domainembedding.NewHashNgramVectorizer()
	}
	return &Service{
		repo:       repo,
		vectorizer: vectorizer,
	}
}

// GenerateForPublishedVideo 生成向量并按视频模型保存；无效事件忽略，序列化或存储失败返回应用错误供消费者重试。
func (s *Service) GenerateForPublishedVideo(ctx context.Context, event *applicationvideo.PublishedEvent) (*GenerateVideoEmbeddingResult, error) {
	if event == nil || event.VideoID <= 0 {
		return &GenerateVideoEmbeddingResult{}, nil
	}

	text := domainembedding.BuildVideoText(event.Title, event.Description)
	vector := s.vectorizer.Vectorize(text)
	content, err := json.Marshal(vector)
	if err != nil {
		return nil, ErrMarshalEmbeddingFailed
	}

	embedding := domainembedding.NewVideoEmbedding(
		event.VideoID,
		s.vectorizer.Model(),
		vector,
		domainembedding.TextHash(text),
		string(content),
	)
	if err := s.repo.SaveVideoEmbedding(ctx, embedding); err != nil {
		return nil, ErrSaveVideoEmbeddingFailed
	}

	return &GenerateVideoEmbeddingResult{
		Embedding:        embedding,
		CreatedOrUpdated: true,
	}, nil
}
