package relation

import (
	"context"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
)

const defaultBackfillVideoLimit = 100
const defaultBackfillInboxMaxLen = 1000

// FollowFeedBackfiller 定义关注后读取作者作品和回填收件箱所需能力。
type FollowFeedBackfiller interface {
	CountFollowers(ctx context.Context, authorID int64) (int, error)
	ListAuthorRecentVideos(ctx context.Context, authorID int64, limit int) ([]*domainfeed.FeedPageItem, error)
	AddInboxItems(ctx context.Context, authorID int64, userIDs []int64, item *domainfeed.FeedPageItem, maxLen int64) error
}

// Option 配置关系服务的可选回填依赖。
type Option func(*Service)

// WithFollowFeedBackfiller 为关注成功后的关注流索引启用回填。
func WithFollowFeedBackfiller(backfiller FollowFeedBackfiller) Option {
	return func(s *Service) {
		s.backfiller = backfiller
	}
}

// backfillFollowFeed 跳过未配置依赖和大作者，逐条回填最多一百条作品，任一步失败立即返回并保留已完成写入。
func (s *Service) backfillFollowFeed(ctx context.Context, userID int64, targetUserID int64) error {
	if s.backfiller == nil {
		return nil
	}
	followerCount, err := s.backfiller.CountFollowers(ctx, targetUserID)
	if err != nil {
		return err
	}
	if followerCount >= domainfeed.BigCreatorFollowerThreshold {
		return nil
	}

	items, err := s.backfiller.ListAuthorRecentVideos(ctx, targetUserID, defaultBackfillVideoLimit)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item == nil {
			continue
		}
		if err := s.backfiller.AddInboxItems(ctx, targetUserID, []int64{userID}, item, defaultBackfillInboxMaxLen); err != nil {
			return err
		}
	}
	return nil
}
