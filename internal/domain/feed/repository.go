package feed

import (
	"context"
)

// Repository 定义 Feed 读取需要的持久化能力。
type Repository interface {
	// ListTimelinePage 按发布时间倒序读取轻量 Feed 页。
	ListTimelinePage(ctx context.Context, cursor *TimelineCursor, limit int) ([]*FeedPageItem, error)
	// ListHotPage 按热度分倒序读取轻量 Feed 页。
	ListHotPage(ctx context.Context, cursor *HotCursor, limit int) ([]*FeedPageItem, error)
	// ListFollowingPage 按有效关注关系读取公开视频，按时间和视频 ID 倒序分页。
	ListFollowingPage(ctx context.Context, viewerID int64, cursor *TimelineCursor, limit int) ([]*FeedPageItem, error)
	// ListFollowingPullAuthorIDs 查询当前关注的大作者。
	ListFollowingPullAuthorIDs(context.Context, int64) ([]int64, error)
	// ListFollowingAuthorIDs 查询当前全部有效关注作者，校验收件箱可见性。
	ListFollowingAuthorIDs(context.Context, int64) ([]int64, error)
	// BatchGetFeedCards 批量读取视频卡片展示字段。
	BatchGetFeedCards(ctx context.Context, videoIDs []int64) (map[int64]*FeedCard, error)
	// BatchGetFeedStats 批量读取视频互动计数。
	BatchGetFeedStats(ctx context.Context, videoIDs []int64) (map[int64]*FeedStat, error)
	// BatchGetViewerActionStates 批量读取当前用户对视频的点赞和收藏状态。
	BatchGetViewerActionStates(ctx context.Context, viewerID int64, videoIDs []int64) (map[int64]*ViewerActionState, error)
}
