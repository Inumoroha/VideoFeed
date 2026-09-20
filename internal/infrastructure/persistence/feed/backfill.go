package feed

import (
	"context"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"
)

// ListAuthorRecentVideos 查询作者最近公开视频，用于新关注小作者时回填当前用户 inbox。
func (r *Repository) ListAuthorRecentVideos(ctx context.Context, authorID int64, limit int) ([]*domainfeed.FeedPageItem, error) {
	if authorID <= 0 || limit <= 0 {
		return []*domainfeed.FeedPageItem{}, nil
	}

	var models []domainfeed.FeedPageItem
	err := r.db.WithContext(ctx).
		Table("videos AS v").
		Select("v.id AS video_id, v.author_id, v.published_at").
		Where("v.author_id = ? AND v.status = ? AND v.published_at IS NOT NULL", authorID, domainvideo.StatusPublished).
		Order("v.published_at DESC").
		Order("v.id DESC").
		Limit(limit).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}
	return feedPageItemsFromModels(models), nil
}
