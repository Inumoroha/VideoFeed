package relation

import (
	"context"
	"errors"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	"testing"
)

// failingBackfiller 注入回填阶段故障并记录调用次数与容量参数。
type failingBackfiller struct {
	stage                                    string
	countCalls, listCalls, writeCalls, limit int
	maxLen                                   int64
}

// CountFollowers 模拟粉丝数读取或故障。
func (b *failingBackfiller) CountFollowers(context.Context, int64) (int, error) {
	b.countCalls++
	if b.stage == "count" {
		return 0, errors.New("failed")
	}
	return 3, nil
}

// ListAuthorRecentVideos 记录页大小并返回含空条目的作品或故障。
func (b *failingBackfiller) ListAuthorRecentVideos(_ context.Context, _ int64, limit int) ([]*domainfeed.FeedPageItem, error) {
	b.listCalls++
	b.limit = limit
	if b.stage == "list" {
		return nil, errors.New("failed")
	}
	return []*domainfeed.FeedPageItem{nil, {VideoID: 1}, {VideoID: 2}}, nil
}

// AddInboxItems 在第二次写入模拟失败以验证部分完成与重试。
func (b *failingBackfiller) AddInboxItems(_ context.Context, _ int64, _ []int64, _ *domainfeed.FeedPageItem, maxLen int64) error {
	b.writeCalls++
	b.maxLen = maxLen
	if b.stage == "write" && b.writeCalls == 2 {
		return errors.New("failed")
	}
	return nil
}

// TestBackfillFailuresAndRetry 验证关系先保存、各阶段失败转换、部分写入后显式重试、取关跳过和仓储失败短路。
func TestBackfillFailuresAndRetry(t *testing.T) {
	for _, stage := range []string{"count", "list", "write"} {
		t.Run(stage, func(t *testing.T) {
			repo := backfillRelationRepo()
			b := &failingBackfiller{stage: stage}
			s := NewService(repo, WithFollowFeedBackfiller(b))
			result, err := s.Follow(context.Background(), 42, 77, "same")
			if !errors.Is(err, ErrBackfillFollowFeedFailed) || result != nil || repo.setFollowCalls != 1 || !repo.follow.Active() {
				t.Fatal(result, err)
			}
			b.stage = ""
			result, err = s.Follow(context.Background(), 42, 77, "same")
			if err != nil || !result.Following || b.limit != 100 || b.maxLen != 1000 || repo.setFollowKey != "same" {
				t.Fatal(result, err, b)
			}
			before := b.countCalls
			if _, err = s.Unfollow(context.Background(), 42, 77, ""); err != nil || b.countCalls != before {
				t.Fatal(err)
			}
			repo.setFollowErr = errors.New("db failed")
			if _, err = s.Follow(context.Background(), 42, 77, ""); !errors.Is(err, ErrUpdateRelationFailed) || b.countCalls != before {
				t.Fatal(err)
			}
		})
	}
}
