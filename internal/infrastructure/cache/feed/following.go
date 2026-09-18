package feed

import (
	"context"
	"fmt"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	"github.com/redis/go-redis/v9"
	"sort"
	"strconv"
	"strings"
	"time"
)

const followingIndexKeyTTL = 30 * 24 * time.Hour

// AddInboxItems 按粉丝写入去重索引，并在事务管道内裁剪容量和刷新有效期。
func (c *FeedCache) AddInboxItems(ctx context.Context, authorID int64, userIDs []int64, item *domainfeed.FeedPageItem, maxLen int64) error {
	if authorID <= 0 || item == nil || item.VideoID <= 0 || item.PublishedAt.IsZero() || len(userIDs) == 0 {
		return nil
	}
	if maxLen <= 0 {
		maxLen = 1000
	}
	pipe := c.client.TxPipeline()
	score := followingIndexScore(item.PublishedAt, item.VideoID)
	member := followingIndexMember(item.VideoID, authorID, item.PublishedAt)
	for _, userID := range userIDs {
		if userID <= 0 {
			continue
		}
		key := followingInboxKey(userID)
		pipe.ZAdd(ctx, key, redis.Z{Score: score, Member: member})
		pipe.ZRemRangeByRank(ctx, key, 0, -maxLen-1)
		pipe.Expire(ctx, key, followingIndexKeyTTL)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// AddAuthorOutboxItem 写入作者去重索引，并在事务管道内裁剪容量和刷新有效期。
func (c *FeedCache) AddAuthorOutboxItem(ctx context.Context, authorID int64, item *domainfeed.FeedPageItem, maxLen int64) error {
	if authorID <= 0 || item == nil || item.VideoID <= 0 || item.PublishedAt.IsZero() {
		return nil
	}
	if maxLen <= 0 {
		maxLen = 500
	}
	key := followingAuthorOutboxKey(authorID)
	score := followingIndexScore(item.PublishedAt, item.VideoID)
	member := followingIndexMember(item.VideoID, authorID, item.PublishedAt)
	pipe := c.client.TxPipeline()
	pipe.ZAdd(ctx, key, redis.Z{Score: score, Member: member})
	pipe.ZRemRangeByRank(ctx, key, 0, -maxLen-1)
	pipe.Expire(ctx, key, followingIndexKeyTTL)
	_, err := pipe.Exec(ctx)
	return err
}

// ListFollowingIndexPage 合并收件箱与当前关注作者发件箱，过滤取关条目，按精确时间和编号去重分页。
func (c *FeedCache) ListFollowingIndexPage(ctx context.Context, viewerID int64, authorIDs []int64, cursor *domainfeed.TimelineCursor, limit int) ([]*domainfeed.FeedPageItem, bool, error) {
	if viewerID <= 0 || limit <= 0 {
		return []*domainfeed.FeedPageItem{}, false, nil
	}
	allowedAuthors := int64Set(authorIDs)
	keys := []string{followingInboxKey(viewerID)}
	for _, authorID := range authorIDs {
		if authorID > 0 {
			keys = append(keys, followingAuthorOutboxKey(authorID))
		}
	}

	pipe := c.client.Pipeline()
	cardinalityCommands := make([]*redis.IntCmd, 0, len(keys))
	rangeCommands := make([]*redis.StringSliceCmd, 0, len(keys))
	for _, key := range keys {
		cardinalityCommands = append(cardinalityCommands, pipe.ZCard(ctx, key))
		rangeCommands = append(rangeCommands, pipe.ZRevRange(ctx, key, 0, -1))
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, false, err
	}

	hasIndex := false
	for _, cmd := range cardinalityCommands {
		count, err := cmd.Result()
		if err != nil && err != redis.Nil {
			return nil, false, err
		}
		if count > 0 {
			hasIndex = true
			break
		}
	}
	if !hasIndex {
		return nil, false, nil
	}

	seen := map[int64]struct{}{}
	items := make([]*domainfeed.FeedPageItem, 0, limit*len(rangeCommands))
	for _, cmd := range rangeCommands {
		members, err := cmd.Result()
		if err != nil && err != redis.Nil {
			return nil, false, err
		}
		for _, member := range members {
			item, ok := feedPageItemFromFollowingMember(member)
			if !ok {
				continue
			}
			if cursor != nil && !item.PublishedAt.Before(cursor.PublishedAt) && !(item.PublishedAt.Equal(cursor.PublishedAt) && item.VideoID < cursor.VideoID) {
				continue
			}
			if item.AuthorID > 0 {
				if _, followed := allowedAuthors[item.AuthorID]; !followed {
					continue
				}
			}
			if _, exists := seen[item.VideoID]; exists {
				continue
			}
			seen[item.VideoID] = struct{}{}
			items = append(items, item)
		}
	}
	sortFeedPageItemsByTimeline(items)
	if len(items) > limit {
		items = items[:limit]
	}
	return items, true, nil
}

// followingInboxKey 生成用户收件箱索引键。
func followingInboxKey(userID int64) string {
	return fmt.Sprintf("feed:following:inbox:v1:%d", userID)
}

// followingAuthorOutboxKey 生成作者发件箱索引键。
func followingAuthorOutboxKey(authorID int64) string {
	return fmt.Sprintf("feed:following:author:v1:%d", authorID)
}

// followingIndexScore 使用微秒发布时间作为排序分值，读取时以完整时间和编号精确排序。
func followingIndexScore(publishedAt time.Time, videoID int64) float64 {
	return float64(publishedAt.UTC().UnixMicro())
}

// followingIndexMember 编码视频、作者和完整发布时间，作为去重成员。
func followingIndexMember(videoID int64, authorID int64, publishedAt time.Time) string {
	return fmt.Sprintf("%d:%d:%s", videoID, authorID, publishedAt.UTC().Format(time.RFC3339Nano))
}

// feedPageItemFromFollowingMember 解码索引成员，拒绝无效编号和时间。
func feedPageItemFromFollowingMember(member string) (*domainfeed.FeedPageItem, bool) {
	parts := strings.SplitN(member, ":", 3)
	if len(parts) != 2 && len(parts) != 3 {
		return nil, false
	}
	videoID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || videoID <= 0 {
		return nil, false
	}
	authorID := int64(0)
	publishedAtIndex := 1
	if len(parts) == 3 {
		authorID, _ = strconv.ParseInt(parts[1], 10, 64)
		publishedAtIndex = 2
	}
	publishedAt, err := time.Parse(time.RFC3339Nano, parts[publishedAtIndex])
	if err != nil || publishedAt.IsZero() {
		return nil, false
	}
	return &domainfeed.FeedPageItem{
		VideoID:     videoID,
		AuthorID:    authorID,
		PublishedAt: publishedAt,
	}, true
}

// int64Set 将编号列表转换为集合。
func int64Set(values []int64) map[int64]struct{} {
	set := map[int64]struct{}{}
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

// sortFeedPageItemsByTimeline 按完整发布时间与视频编号倒序排序。
func sortFeedPageItemsByTimeline(items []*domainfeed.FeedPageItem) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].PublishedAt.Equal(items[j].PublishedAt) {
			return items[i].VideoID > items[j].VideoID
		}
		return items[i].PublishedAt.After(items[j].PublishedAt)
	})
}
