package recommendation

import (
	"context"
	"errors"
	domain "github.com/inumoroha/video-feed/internal/domain/recommendation"
	"math"
	"testing"
	"time"
)

// stubRepository 为推荐服务注入候选、画像和指定阶段的失败。
type stubRepository struct {
	pool     []*domain.Candidate
	vectors  map[int64][]float64
	interest []float64
	fail     string
	calls    int
	writes   []*domain.ExposureWrite
	limit    int
}

// ListCandidatePool 记录候选池大小并返回候选或模拟故障。
func (r *stubRepository) ListCandidatePool(_ context.Context, _ int64, intLimit int) ([]*domain.Candidate, error) {
	r.limit = intLimit
	if r.fail == "pool" {
		return nil, errors.New("pool")
	}
	return r.pool, nil
}

// LoadVideoVectors 返回测试向量或模拟故障。
func (r *stubRepository) LoadVideoVectors(context.Context, []int64) (map[int64][]float64, error) {
	if r.fail == "vectors" {
		return nil, errors.New("vectors")
	}
	return r.vectors, nil
}

// LoadUserInterestVector 返回测试画像或模拟故障。
func (r *stubRepository) LoadUserInterestVector(context.Context, int64) ([]float64, bool, error) {
	if r.fail == "interest" {
		return nil, false, errors.New("interest")
	}
	return r.interest, len(r.interest) > 0, nil
}

// SaveExposures 记录调用及曝光批次，按需模拟不存在或保存失败。
func (r *stubRepository) SaveExposures(_ context.Context, writes []*domain.ExposureWrite) ([]*domain.Exposure, error) {
	r.calls++
	r.writes = writes
	if r.fail == "missing" {
		return nil, domain.ErrVideoNotFound
	}
	if r.fail == "save" {
		return nil, errors.New("save")
	}
	return []*domain.Exposure{}, nil
}

// TestRecommendationFailuresAndRetry 验证读取各阶段失败统一转换，恢复依赖后重试成功且无隐式重试。
func TestRecommendationFailuresAndRetry(t *testing.T) {
	for _, stage := range []string{"pool", "vectors", "interest"} {
		t.Run(stage, func(t *testing.T) {
			r := &stubRepository{pool: []*domain.Candidate{{VideoID: 1, PublishedAt: time.Now()}}, fail: stage}
			s := New(r)
			req := CandidateRequest{UserID: 42, Scene: "recommend"}
			if _, err := s.Recommend(context.Background(), req); !errors.Is(err, ErrLoadRecommendationFailed) {
				t.Fatal(err)
			}
			r.fail = ""
			result, err := s.Recommend(context.Background(), req)
			if err != nil || len(result.Candidates) != 1 {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
}

// TestExposureValidationDedupAndRetry 验证空批次、先校验后写入、批内去重、错误映射与显式重试。
func TestExposureValidationDedupAndRetry(t *testing.T) {
	r := &stubRepository{}
	s := New(r)
	ctx := context.Background()
	result, err := s.SaveExposures(ctx, nil)
	if err != nil || len(result.Exposures) != 0 || r.calls != 0 {
		t.Fatal(result, err)
	}
	inputs := []ExposureInput{{UserID: 42, VideoID: 1, Scene: " Recommend ", RequestID: " same "}, {UserID: 42, VideoID: 1, Scene: "recommend"}}
	if _, err = s.SaveExposures(ctx, append(inputs, ExposureInput{})); !errors.Is(err, domain.ErrInvalidUserID) || r.calls != 0 {
		t.Fatal(err, r.calls)
	}
	if _, err = s.SaveExposures(ctx, inputs); err != nil || len(r.writes) != 1 || r.writes[0].Scene != "recommend" || r.writes[0].RequestID != "same" {
		t.Fatal(err, r.writes)
	}
	for _, stage := range []string{"missing", "save"} {
		r.fail = stage
		_, err = s.SaveExposures(ctx, inputs)
		want := ErrSaveRecommendationExposureFailed
		if stage == "missing" {
			want = domain.ErrVideoNotFound
		}
		if !errors.Is(err, want) {
			t.Fatal(err)
		}
	}
	r.fail = ""
	before := r.calls
	if _, err = s.SaveExposures(ctx, inputs); err != nil || r.calls != before+1 {
		t.Fatal(err, r.calls)
	}
}

// TestRankingFallbackAndPagination 验证冷启动、缺失或错误维度向量回退，作者打散前生成游标且不修改仓储候选。
func TestRankingFallbackAndPagination(t *testing.T) {
	now := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	r := &stubRepository{pool: []*domain.Candidate{{VideoID: 4, AuthorID: 1, PublishedAt: now}, {VideoID: 3, AuthorID: 1, PublishedAt: now}, {VideoID: 2, AuthorID: 2, PublishedAt: now}, {VideoID: 1, AuthorID: 3, PublishedAt: now}}, vectors: map[int64][]float64{4: {1, 0, 0}}}
	s := New(r, WithNow(func() time.Time { return now }))
	req := CandidateRequest{UserID: 42, Scene: "recommend", Limit: 3}
	result, err := s.Recommend(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if r.limit != 50 || !result.HasMore || result.Candidates[0].VideoID != 4 || result.Candidates[1].VideoID != 2 || result.Candidates[2].VideoID != 3 {
		t.Fatal(result)
	}
	cursor, err := parseCursor(result.NextCursor)
	if err != nil || cursor.VideoID != 2 {
		t.Fatal(cursor, err)
	}
	req.Cursor = result.NextCursor
	next, err := s.Recommend(context.Background(), req)
	if err != nil || len(next.Candidates) != 1 || next.Candidates[0].VideoID != 1 || next.HasMore {
		t.Fatal(next, err)
	}
	if r.pool[0].RankScore != 0 {
		t.Fatal("仓储候选被修改")
	}
	r.interest = []float64{1, 0}
	req.Cursor = ""
	result, err = s.Recommend(context.Background(), req)
	if err != nil || result.Candidates[0].Similarity != 0 || result.Candidates[0].Reason != "fresh" {
		t.Fatal(result, err)
	}
	if math.Abs(rankScore(1, 0, 1, true)-0.8) > 1e-9 || math.Abs(rankScore(0, 0, 1, false)-0.35) > 1e-9 {
		t.Fatal("评分权重改变")
	}
	if freshnessScore(now, time.Time{}) != 0 || freshnessScore(now, now.Add(time.Hour)) != 1 {
		t.Fatal("新鲜度边界错误")
	}
	if candidatePoolLimit(100) != 500 || normalizeLimit(-1) != 10 || normalizeLimit(101) != 100 {
		t.Fatal("分页限制错误")
	}
}

// TestCursorBoundaries 验证坏游标拒绝、空游标及无效游标编码。
func TestCursorBoundaries(t *testing.T) {
	for _, raw := range []string{"!", "e30", "bnVsbA"} {
		if _, err := parseCursor(raw); !errors.Is(err, domain.ErrInvalidCursor) {
			t.Fatal(raw, err)
		}
	}
	if c, err := parseCursor(" "); c != nil || err != nil {
		t.Fatal(c, err)
	}
	if encodeCursor(nil) != "" || encodeCursor(&domain.Cursor{RankScore: math.Inf(1)}) != "" {
		t.Fatal("无效游标编码")
	}
}
