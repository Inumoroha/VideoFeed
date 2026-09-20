package relation

import (
	"context"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	domainrelation "github.com/inumoroha/video-feed/internal/domain/relation"
	"sync"
	"testing"
	"time"
)

// memoryFollowFeedBackfiller 通过互斥锁保护测试作者和回填写入。
type memoryFollowFeedBackfiller struct {
	mu            sync.Mutex
	followerCount int
	items         []*domainfeed.FeedPageItem
	writes        []backfillWrite
}

// backfillWrite 记录回填的作者、用户、视频和容量。
type backfillWrite struct {
	AuthorID int64
	UserIDs  []int64
	VideoID  int64
	MaxLen   int64
}

// TestRelationFollowBackfillsSmallCreatorInbox 覆盖新关注小作者后把近期作品回填到当前用户 inbox。
func TestRelationFollowBackfillsSmallCreatorInbox(t *testing.T) {
	repo := backfillRelationRepo()
	backfiller := &memoryFollowFeedBackfiller{
		followerCount: 3,
		items: []*domainfeed.FeedPageItem{
			{VideoID: 101, AuthorID: 77, PublishedAt: time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)},
			{VideoID: 100, AuthorID: 77, PublishedAt: time.Date(2026, 5, 4, 11, 0, 0, 0, time.UTC)},
		},
	}
	service := NewService(repo, WithFollowFeedBackfiller(backfiller))

	result, err := service.Follow(context.Background(), 42, 77, "follow-backfill")
	if err != nil {
		t.Fatalf("follow with backfill: %v", err)
	}
	if !result.Following {
		t.Fatalf("expected active follow: %+v", result)
	}

	writes := backfiller.Writes()
	if len(writes) != 2 {
		t.Fatalf("unexpected backfill writes: %+v", writes)
	}
	if writes[0].AuthorID != 77 || writes[0].VideoID != 101 || len(writes[0].UserIDs) != 1 || writes[0].UserIDs[0] != 42 {
		t.Fatalf("unexpected first backfill write: %+v", writes[0])
	}
	if writes[0].MaxLen != 1000 {
		t.Fatalf("unexpected inbox max len: %d", writes[0].MaxLen)
	}
}

// TestRelationFollowSkipsBigCreatorInboxBackfill 覆盖大作者关注流继续走 outbox 拉模式。
func TestRelationFollowSkipsBigCreatorInboxBackfill(t *testing.T) {
	repo := backfillRelationRepo()
	backfiller := &memoryFollowFeedBackfiller{
		followerCount: domainfeed.BigCreatorFollowerThreshold,
		items: []*domainfeed.FeedPageItem{
			{VideoID: 101, AuthorID: 77, PublishedAt: time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)},
		},
	}
	service := NewService(repo, WithFollowFeedBackfiller(backfiller))

	if _, err := service.Follow(context.Background(), 42, 77, "follow-big"); err != nil {
		t.Fatalf("follow big creator: %v", err)
	}
	if len(backfiller.Writes()) != 0 {
		t.Fatalf("unexpected big creator inbox backfill: %+v", backfiller.Writes())
	}
}

// CountFollowers 加锁读取测试粉丝数。
func (b *memoryFollowFeedBackfiller) CountFollowers(ctx context.Context, authorID int64) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.followerCount, nil
}

// ListAuthorRecentVideos 加锁复制作者作品并限制数量。
func (b *memoryFollowFeedBackfiller) ListAuthorRecentVideos(ctx context.Context, authorID int64, limit int) ([]*domainfeed.FeedPageItem, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	items := make([]*domainfeed.FeedPageItem, 0, len(b.items))
	for _, item := range b.items {
		if item == nil || item.AuthorID != authorID {
			continue
		}
		value := *item
		items = append(items, &value)
		if limit > 0 && len(items) >= limit {
			break
		}
	}
	return items, nil
}

// AddInboxItems 加锁保存回填写入及用户列表副本。
func (b *memoryFollowFeedBackfiller) AddInboxItems(ctx context.Context, authorID int64, userIDs []int64, item *domainfeed.FeedPageItem, maxLen int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.writes = append(b.writes, backfillWrite{
		AuthorID: authorID,
		UserIDs:  append([]int64(nil), userIDs...),
		VideoID:  item.VideoID,
		MaxLen:   maxLen,
	})
	return nil
}

// Writes 加锁返回回填写入的深拷贝。
func (b *memoryFollowFeedBackfiller) Writes() []backfillWrite {
	b.mu.Lock()
	defer b.mu.Unlock()
	writes := make([]backfillWrite, 0, len(b.writes))
	for _, write := range b.writes {
		write.UserIDs = append([]int64(nil), write.UserIDs...)
		writes = append(writes, write)
	}
	return writes
}

// backfillRelationRepo 构造已关注作者的仓储响应，供回填测试复用。
func backfillRelationRepo() *relationServiceRepository {
	return &relationServiceRepository{
		follow:     domainrelation.RestoreFollow(1, 42, 77, domainrelation.FollowStatusActive, "", time.Time{}, time.Time{}),
		userStat:   domainrelation.RestoreRelationStat(42, 1, 0, time.Time{}, time.Time{}),
		targetStat: domainrelation.RestoreRelationStat(77, 0, 3, time.Time{}, time.Time{}),
	}
}
