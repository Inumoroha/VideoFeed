package feed

import (
	"context"
	"errors"
	recommendation "github.com/inumoroha/video-feed/internal/application/recommendation"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	domainrecommendation "github.com/inumoroha/video-feed/internal/domain/recommendation"
	"testing"
	"time"
)

// recommendationStub 记录推荐输入并返回可配置结果或错误。
type recommendationStub struct {
	request recommendation.CandidateRequest
	err     error
}

// Recommend 记录用户和客户端上下文并返回顺序固定的候选。
func (r *recommendationStub) Recommend(_ context.Context, req recommendation.CandidateRequest) (*recommendation.CandidateResult, error) {
	r.request = req
	if r.err != nil {
		return nil, r.err
	}
	return &recommendation.CandidateResult{Candidates: []*domainrecommendation.Candidate{{VideoID: 2, AuthorID: 7, PublishedAt: time.Now()}, {VideoID: 1, AuthorID: 8, PublishedAt: time.Now()}}, NextCursor: "next", HasMore: true}, nil
}

// failingRecommendationCache 模拟公共卡片和计数缓存故障，其余缓存能力不得用于个性化页。
type failingRecommendationCache struct{ FeedCache }

// GetCards 模拟卡片缓存不可用以触发仓储回源。
func (failingRecommendationCache) GetCards(context.Context, []int64) (map[int64]*domainfeed.FeedCard, error) {
	return nil, errors.New("cache down")
}

// GetStats 模拟计数缓存不可用以触发仓储回源。
func (failingRecommendationCache) GetStats(context.Context, []int64) (map[int64]*domainfeed.FeedStat, error) {
	return nil, errors.New("cache down")
}

// SetCards 模拟回填失败，确保不改变成功响应。
func (failingRecommendationCache) SetCards(context.Context, map[int64]*domainfeed.FeedCard, time.Duration) error {
	return errors.New("cache down")
}

// SetStats 模拟计数回填失败，确保不改变成功响应。
func (failingRecommendationCache) SetStats(context.Context, map[int64]*domainfeed.FeedStat, time.Duration) error {
	return errors.New("cache down")
}

// TestRecommendStrategyCacheFallback 验证推荐身份与上下文传递、缓存故障回源、卡片顺序及各类错误映射。
func TestRecommendStrategyCacheFallback(t *testing.T) {
	r := &recommendationCardRepository{}
	rec := &recommendationStub{}
	s := NewService(r, WithRecommender(rec), WithFeedCache(failingRecommendationCache{}))
	req := FeedRequest{Scene: domainfeed.SceneRecommend, ViewerID: 42, Limit: 2, ClientContext: map[string]string{"request_id": "req"}}
	result, err := s.GetFeed(context.Background(), req)
	if err != nil || len(result.Items) != 2 || result.Items[0].VideoID != 2 || result.Items[1].VideoID != 1 || result.NextCursor != "next" || !result.HasMore {
		t.Fatal(result, err)
	}
	if rec.request.UserID != 42 || rec.request.RequestID != "req" || rec.request.Scene != "recommend" {
		t.Fatal(rec.request)
	}
	req.ViewerID = 0
	if _, err = s.GetFeed(context.Background(), req); !errors.Is(err, domainfeed.ErrViewerRequired) {
		t.Fatal(err)
	}
	req.ViewerID = 42
	rec.err = recommendation.ErrLoadRecommendationFailed
	if _, err = s.GetFeed(context.Background(), req); !errors.Is(err, ErrLoadFeedFailed) {
		t.Fatal(err)
	}
	rec.err = domainrecommendation.ErrInvalidCursor
	if _, err = s.GetFeed(context.Background(), req); !errors.Is(err, rec.err) {
		t.Fatal(err)
	}
	rec.err = nil
	r.err = errors.New("cards failed")
	if _, err = s.GetFeed(context.Background(), req); !errors.Is(err, ErrLoadFeedFailed) {
		t.Fatal(err)
	}
}

// recommendationCardRepository 为推荐卡片回源注入失败。
type recommendationCardRepository struct{ serviceTestRepository }

// BatchGetFeedCards 返回卡片或指定的仓储错误。
func (r *recommendationCardRepository) BatchGetFeedCards(ctx context.Context, ids []int64) (map[int64]*domainfeed.FeedCard, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.serviceTestRepository.BatchGetFeedCards(ctx, ids)
}
