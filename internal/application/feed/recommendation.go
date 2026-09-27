package feed

import (
	"context"
	"errors"
	applicationrecommendation "github.com/inumoroha/video-feed/internal/application/recommendation"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
)

// RecommendStrategy 使用推荐服务读取排序后的候选，再复用 Feed 卡片组装。
type RecommendStrategy struct {
	repo        domainfeed.Repository
	cache       FeedCache
	recommender Recommender
}

// Recommender 定义推荐 Feed 使用的候选查询能力。
type Recommender interface {
	Recommend(ctx context.Context, input applicationrecommendation.CandidateRequest) (*applicationrecommendation.CandidateResult, error)
}

// WithRecommender 注册推荐 Feed 策略。
func WithRecommender(recommender Recommender) Option {
	return func(s *Service) {
		if recommender != nil {
			s.RegisterStrategy(NewRecommendStrategy(s.repo, recommender))
		}
	}
}

// NewRecommendStrategy 创建推荐 Feed 策略。
func NewRecommendStrategy(repo domainfeed.Repository, recommender Recommender) *RecommendStrategy {
	return &RecommendStrategy{
		repo:        repo,
		recommender: recommender,
	}
}

// Scene 返回推荐场景。
func (s *RecommendStrategy) Scene() domainfeed.Scene {
	return domainfeed.SceneRecommend
}

// List 读取推荐候选，并按推荐服务给出的顺序组装 Feed 卡片。
func (s *RecommendStrategy) List(ctx context.Context, req FeedRequest) (*FeedResult, error) {
	if req.ViewerID <= 0 {
		return nil, domainfeed.ErrViewerRequired
	}
	limit := normalizeLimit(req.Limit)
	result, err := s.recommender.Recommend(ctx, applicationrecommendation.CandidateRequest{
		UserID:    req.ViewerID,
		Scene:     string(domainfeed.SceneRecommend),
		RequestID: clientContextValue(req.ClientContext, "request_id"),
		Cursor:    req.Cursor,
		Limit:     limit,
	})
	if err != nil {
		if errors.Is(err, applicationrecommendation.ErrLoadRecommendationFailed) {
			return nil, ErrLoadFeedFailed
		}
		return nil, err
	}

	pageItems := make([]*domainfeed.FeedPageItem, 0, len(result.Candidates))
	for _, candidate := range result.Candidates {
		pageItems = append(pageItems, &domainfeed.FeedPageItem{
			VideoID:     candidate.VideoID,
			AuthorID:    candidate.AuthorID,
			PublishedAt: candidate.PublishedAt,
			HotScore:    candidate.HotScore,
		})
	}
	items, err := assembleFeedItems(ctx, s.repo, s.cache, pageItems, req.ViewerID)
	if err != nil {
		return nil, ErrLoadFeedFailed
	}
	return &FeedResult{
		Scene:      domainfeed.SceneRecommend,
		Items:      items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}, nil
}

// clientContextValue 读取客户端上下文指定字段，空映射返回空字符串。
func clientContextValue(context map[string]string, key string) string {
	if context == nil {
		return ""
	}
	return context[key]
}
