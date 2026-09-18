package feed

import (
	"context"
	applicationfeed "github.com/inumoroha/video-feed/internal/application/feed"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	"net/http"
	"sort"
	"sync"
	"testing"
	"time"
)

// memoryFeedCache 以互斥锁保存测试页、卡片、统计及关注索引。
type memoryFeedCache struct {
	mu             sync.Mutex
	pages          map[string]*applicationfeed.FeedPage
	cards          map[int64]*domainfeed.FeedCard
	stats          map[int64]*domainfeed.FeedStat
	hotItems       []*domainfeed.FeedPageItem
	followingInbox map[int64][]*domainfeed.FeedPageItem
	authorOutbox   map[int64][]*domainfeed.FeedPageItem
}

// newMemoryFeedCache 初始化测试缓存及关注索引容器。
func newMemoryFeedCache() *memoryFeedCache {
	return &memoryFeedCache{
		pages:          map[string]*applicationfeed.FeedPage{},
		cards:          map[int64]*domainfeed.FeedCard{},
		stats:          map[int64]*domainfeed.FeedStat{},
		followingInbox: map[int64][]*domainfeed.FeedPageItem{},
		authorOutbox:   map[int64][]*domainfeed.FeedPageItem{},
	}
}

// GetPage 加锁读取并复制测试缓存页。
func (c *memoryFeedCache) GetPage(ctx context.Context, key string) (*applicationfeed.FeedPage, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	page, ok := c.pages[key]
	if !ok {
		return nil, false, nil
	}
	cloned := *page
	cloned.Items = cloneFeedPageItems(page.Items)
	return &cloned, true, nil
}

// SetPage 加锁保存测试缓存页的副本。
func (c *memoryFeedCache) SetPage(ctx context.Context, key string, page *applicationfeed.FeedPage, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	cloned := *page
	cloned.Items = cloneFeedPageItems(page.Items)
	c.pages[key] = &cloned
	return nil
}

// GetCards 加锁读取并复制指定视频的缓存卡片。
func (c *memoryFeedCache) GetCards(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedCard, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cards := map[int64]*domainfeed.FeedCard{}
	for _, videoID := range videoIDs {
		if card := c.cards[videoID]; card != nil {
			cloned := *card
			cards[videoID] = &cloned
		}
	}
	return cards, nil
}

// SetCards 加锁保存非空视频卡片的副本。
func (c *memoryFeedCache) SetCards(ctx context.Context, cards map[int64]*domainfeed.FeedCard, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for videoID, card := range cards {
		if card == nil {
			continue
		}
		cloned := *card
		c.cards[videoID] = &cloned
	}
	return nil
}

// GetStats 加锁读取并复制指定视频的缓存统计。
func (c *memoryFeedCache) GetStats(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	stats := map[int64]*domainfeed.FeedStat{}
	for _, videoID := range videoIDs {
		if stat := c.stats[videoID]; stat != nil {
			cloned := *stat
			stats[videoID] = &cloned
		}
	}
	return stats, nil
}

// SetStats 加锁保存非空视频统计的副本。
func (c *memoryFeedCache) SetStats(ctx context.Context, stats map[int64]*domainfeed.FeedStat, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for videoID, stat := range stats {
		if stat == nil {
			continue
		}
		cloned := *stat
		c.stats[videoID] = &cloned
	}
	return nil
}

// ListHotWindowPage 按偏移量读取测试热榜缓存页。
func (c *memoryFeedCache) ListHotWindowPage(ctx context.Context, windowEnd time.Time, offset int, limit int) ([]*domainfeed.FeedPageItem, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	items := cloneFeedPageItems(c.hotItems)
	if offset < 0 {
		offset = 0
	}
	if offset >= len(items) || limit <= 0 {
		return []*domainfeed.FeedPageItem{}, nil
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end], nil
}

// ListFollowingIndexPage 合并测试收件箱和作者发件箱，去重排序后按游标分页。
func (c *memoryFeedCache) ListFollowingIndexPage(ctx context.Context, viewerID int64, authorIDs []int64, cursor *domainfeed.TimelineCursor, limit int) ([]*domainfeed.FeedPageItem, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	items := make([]*domainfeed.FeedPageItem, 0)
	items = append(items, cloneFeedPageItems(c.followingInbox[viewerID])...)
	for _, authorID := range authorIDs {
		items = append(items, cloneFeedPageItems(c.authorOutbox[authorID])...)
	}
	if len(items) == 0 {
		return nil, false, nil
	}
	items = filterAndSortTimelineItems(items, cursor)
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	return items, true, nil
}

// AddInboxItemsForTest 加锁将测试条目副本加入指定用户收件箱。
func (c *memoryFeedCache) AddInboxItemsForTest(userIDs []int64, item *domainfeed.FeedPageItem) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, userID := range userIDs {
		c.followingInbox[userID] = append(c.followingInbox[userID], cloneFeedPageItems([]*domainfeed.FeedPageItem{item})...)
	}
}

// AddAuthorOutboxItemForTest 加锁将测试条目副本加入作者发件箱。
func (c *memoryFeedCache) AddAuthorOutboxItemForTest(authorID int64, item *domainfeed.FeedPageItem) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.authorOutbox[authorID] = append(c.authorOutbox[authorID], cloneFeedPageItems([]*domainfeed.FeedPageItem{item})...)
}

// cloneFeedPageItems 复制非空轻量条目，隔离测试缓存引用。
func cloneFeedPageItems(items []*domainfeed.FeedPageItem) []*domainfeed.FeedPageItem {
	cloned := make([]*domainfeed.FeedPageItem, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		value := *item
		cloned = append(cloned, &value)
	}
	return cloned
}

// filterAndSortTimelineItems 按游标过滤并去重，按发布时间与编号倒序排序。
func filterAndSortTimelineItems(items []*domainfeed.FeedPageItem, cursor *domainfeed.TimelineCursor) []*domainfeed.FeedPageItem {
	filtered := make([]*domainfeed.FeedPageItem, 0, len(items))
	seen := map[int64]struct{}{}
	for _, item := range items {
		if item == nil {
			continue
		}
		if _, exists := seen[item.VideoID]; exists {
			continue
		}
		if cursor != nil && !item.PublishedAt.Before(cursor.PublishedAt) && !(item.PublishedAt.Equal(cursor.PublishedAt) && item.VideoID < cursor.VideoID) {
			continue
		}
		seen[item.VideoID] = struct{}{}
		value := *item
		filtered = append(filtered, &value)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].PublishedAt.Equal(filtered[j].PublishedAt) {
			return filtered[i].VideoID > filtered[j].VideoID
		}
		return filtered[i].PublishedAt.After(filtered[j].PublishedAt)
	})
	return filtered
}

// TestFollowingFeedUsesRedisIndex 验证关注索引合并排序与分页，命中时不查询数据库页。
func TestFollowingFeedUsesRedisIndex(t *testing.T) {
	repo := newMemoryFeedRepo(seedFollowingFeedItems())
	repo.FollowForTest(42, 100)
	repo.FollowForTest(42, 200)
	repo.SetFollowerCountForTest(100, domainfeed.BigCreatorFollowerThreshold)
	cache := newMemoryFeedCache()
	cache.AddInboxItemsForTest([]int64{42}, &domainfeed.FeedPageItem{
		VideoID:     2,
		PublishedAt: seedFollowingFeedItems()[1].PublishedAt,
	})
	cache.AddAuthorOutboxItemForTest(100, &domainfeed.FeedPageItem{
		VideoID:     4,
		PublishedAt: seedFollowingFeedItems()[3].PublishedAt,
	})
	cache.AddAuthorOutboxItemForTest(100, &domainfeed.FeedPageItem{
		VideoID:     1,
		PublishedAt: seedFollowingFeedItems()[0].PublishedAt,
	})

	router, jwtManager := newFeedRouterWithServiceAndJWT(t, applicationfeed.New(repo, applicationfeed.WithFeedCache(cache)))
	token := signTestToken(t, jwtManager, 42)

	response := performJSONRequest(router, http.MethodGet, "/api/feed-items?scene=following&limit=2", "", token)
	requireStatus(t, response, http.StatusOK)

	var page feedAPIResponse
	decodeJSON(t, response, &page)
	if len(page.Items) != 2 || page.Items[0].VideoID != 4 || page.Items[1].VideoID != 2 || !page.HasMore {
		t.Fatalf("unexpected following index page: %+v", page)
	}
	if repo.FollowingCalls() != 0 || repo.FollowingPullAuthorCalls() != 1 {
		t.Fatalf("unexpected following repo calls: page=%d authors=%d", repo.FollowingCalls(), repo.FollowingPullAuthorCalls())
	}
}
