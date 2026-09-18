package feed

import (
	"context"
	"errors"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	"testing"
)

// brokenFollowingIndex 模拟索引连接故障、缺失或不足一页。
type brokenFollowingIndex struct {
	err error
	hit bool
}

// ListFollowingIndexPage 返回预设故障或不足一页的索引。
func (c brokenFollowingIndex) ListFollowingIndexPage(context.Context, int64, []int64, *domainfeed.TimelineCursor, int) ([]*domainfeed.FeedPageItem, bool, error) {
	return nil, c.hit, c.err
}

// TestFollowingIndexFallback 验证 Redis 故障、未命中和索引不足时仍由数据库返回当前用户视频。
func TestFollowingIndexFallback(t *testing.T) {
	for _, index := range []brokenFollowingIndex{{err: errors.New("redis down")}, {}, {hit: true}} {
		s := NewFollowingStrategy(&followingTestRepository{})
		s.followingIndex = index
		items, err := s.listFollowingItems(context.Background(), 42, nil, 2)
		if err != nil || len(items) != 1 || items[0].VideoID != 42 {
			t.Fatalf("items=%+v err=%v", items, err)
		}
	}
}
