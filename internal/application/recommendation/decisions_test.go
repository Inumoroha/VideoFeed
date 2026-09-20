package recommendation

import (
	"context"
	"errors"
	domain "github.com/inumoroha/video-feed/internal/domain/recommendation"
	"reflect"
	"testing"
	"time"
)

// ListRecentExposures 为既有推荐测试补齐只读仓储能力，默认返回空列表。
func (r *stubRepository) ListRecentExposures(context.Context, int64, []int64, time.Time) ([]*domain.Exposure, error) {
	return []*domain.Exposure{}, nil
}

// decisionRepository 记录查询参数并模拟曝光读取结果、取消和失败，其他调用通过空接口暴露为测试错误。
type decisionRepository struct {
	domain.Repository
	items   []*domain.Exposure
	failure error
	calls   int
	userID  int64
	ids     []int64
	since   time.Time
}

// ListRecentExposures 记录用户、视频与时间边界，优先传播上下文取消，否则返回注入结果。
func (r *decisionRepository) ListRecentExposures(ctx context.Context, userID int64, ids []int64, since time.Time) ([]*domain.Exposure, error) {
	r.calls++
	r.userID, r.ids, r.since = userID, append([]int64(nil), ids...), since
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return r.items, r.failure
}

// TestDecideExposuresReadAndRetry 验证时间窗口、去重顺序、空条目、时间复制、失败转换、显式重试与取消。
func TestDecideExposuresReadAndRetry(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 34, 56, 0, time.UTC)
	exposed := &domain.Exposure{VideoID: 3, LastExposedAt: now.Add(-time.Hour)}
	repo := &decisionRepository{items: []*domain.Exposure{nil, exposed}}
	s := New(repo, WithNow(func() time.Time { return now }))
	input := ExposureDecisionInput{UserID: 42, Scene: " Recommend ", RequestID: " req ", VideoIDs: []int64{3, 2, 3, 1}}
	result, err := s.DecideExposures(context.Background(), input)
	if err != nil || result.UserID != 42 || result.Scene != "recommend" || result.RequestID != "req" || len(result.Decisions) != 3 {
		t.Fatal(result, err)
	}
	if repo.userID != 42 || !reflect.DeepEqual(repo.ids, []int64{3, 2, 1}) || !repo.since.Equal(now.Add(-7*24*time.Hour)) {
		t.Fatal(repo)
	}
	first := result.Decisions[0]
	if first.VideoID != 3 || first.Allowed || first.Reason != domain.ExposureDecisionReasonRecentlyExposed || first.LastExposedAt == nil || !first.LastExposedAt.Equal(exposed.LastExposedAt) {
		t.Fatal(first)
	}
	exposed.LastExposedAt = now
	if first.LastExposedAt.Equal(now) {
		t.Fatal("决策共享了仓储时间指针")
	}
	for i, id := range []int64{2, 1} {
		item := result.Decisions[i+1]
		if item.VideoID != id || !item.Allowed || item.Reason != domain.ExposureDecisionReasonFresh || item.LastExposedAt != nil {
			t.Fatal(item)
		}
	}
	repo.failure = errors.New("database unavailable")
	if _, err = s.DecideExposures(context.Background(), input); !errors.Is(err, ErrLoadExposureDecisionsFailed) || repo.calls != 2 {
		t.Fatal(err, repo.calls)
	}
	repo.failure = nil
	if _, err = s.DecideExposures(context.Background(), input); err != nil || repo.calls != 3 {
		t.Fatal(err, repo.calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.DecideExposures(ctx, input); !errors.Is(err, ErrLoadExposureDecisionsFailed) {
		t.Fatal(err)
	}
}

// TestDecideExposuresEmptyAndInvalid 验证空列表仍校验元数据，合法空列表返回非空切片且不调用仓储。
func TestDecideExposuresEmptyAndInvalid(t *testing.T) {
	s := New(nil)
	result, err := s.DecideExposures(context.Background(), ExposureDecisionInput{UserID: 42, Scene: "recommend"})
	if err != nil || result.Decisions == nil || len(result.Decisions) != 0 {
		t.Fatal(result, err)
	}
	for _, input := range []ExposureDecisionInput{{}, {UserID: 42}, {UserID: 42, Scene: "recommend", VideoIDs: []int64{1, 0}}} {
		if _, err := s.DecideExposures(context.Background(), input); err == nil {
			t.Fatal(input)
		}
	}
}
