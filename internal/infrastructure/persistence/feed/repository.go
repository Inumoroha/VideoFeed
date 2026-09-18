package feed

import (
	"context"
	"errors"
	"fmt"

	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	domainrelation "github.com/inumoroha/video-feed/internal/domain/relation"
	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"
	"gorm.io/gorm"
)

const hotScoreExpression = "COALESCE(vs.like_count, 0) * 3 + COALESCE(vs.comment_count, 0) * 5 + COALESCE(vs.favorite_count, 0) * 4"

// Repository 使用 GORM 持久化信息流数据。
type Repository struct {
	db *gorm.DB
}

var _ domainfeed.Repository = (*Repository)(nil)

// New 创建基于 GORM 数据库实例的信息流仓储。
func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ListTimelinePage 查询时间线 Feed 轻量页，卡片和计数由应用层批量组装。
func (r *Repository) ListTimelinePage(ctx context.Context, cursor *domainfeed.TimelineCursor, limit int) ([]*domainfeed.FeedPageItem, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("feed repository is not initialized")
	}
	if limit <= 0 {
		return nil, domainfeed.ErrInvalidLimit
	}
	if limit > domainfeed.MaxLimit+1 {
		limit = domainfeed.MaxLimit + 1
	}
	if cursor != nil && (cursor.PublishedAt.IsZero() || cursor.VideoID <= 0) {
		return nil, domainfeed.ErrInvalidCursor
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var models []domainfeed.FeedPageItem
	// query := r.db.WithContext(ctx).
	// 	Table("videos AS v").
	// 	Select("v.id AS video_id, v.author_id, COALESCE(u.nickname, '') AS author_nickname, COALESCE(u.avatar_url, '') AS author_avatar_url, v.title, v.description, v.media_url, v.cover_url, COALESCE(vs.like_count, 0) AS like_count, COALESCE(vs.comment_count, 0) AS comment_count, COALESCE(vs.favorite_count, 0) AS favorite_count, v.published_at").
	// 	Joins("LEFT JOIN users AS u ON u.id = v.author_id").
	// 	Joins("LEFT JOIN video_stat AS vs ON vs.video_id = v.id").
	// 	Where("v.status = ? AND v.published_at IS NOT NULL", domainvideo.StatusPublished)
	query := r.basePageQuery(ctx)

	if cursor != nil {
		query = query.Where(
			"(v.published_at < ? OR (v.published_at = ? AND v.id < ?))",
			cursor.PublishedAt,
			cursor.PublishedAt,
			cursor.VideoID,
		)
	}

	err := query.
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

// ListHotPage 查询热榜 Feed 轻量页，按互动热度倒序稳定分页。
func (r *Repository) ListHotPage(ctx context.Context, cursor *domainfeed.HotCursor, limit int) ([]*domainfeed.FeedPageItem, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("feed repository is not initialized")
	}
	if limit <= 0 {
		return nil, domainfeed.ErrInvalidLimit
	}
	if limit > domainfeed.MaxLimit+1 {
		limit = domainfeed.MaxLimit + 1
	}
	if cursor != nil && (cursor.PublishedAt.IsZero() || cursor.VideoID <= 0) {
		return nil, domainfeed.ErrInvalidCursor
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var models []domainfeed.FeedPageItem
	query := r.baseHotPageQuery(ctx)

	if cursor != nil {
		query = query.Where(
			fmt.Sprintf("((%[1]s) < ? OR ((%[1]s) = ? AND v.published_at < ?) OR ((%[1]s) = ? AND v.published_at = ? AND v.id < ?))", hotScoreExpression),
			cursor.HotScore,
			cursor.HotScore,
			cursor.PublishedAt,
			cursor.HotScore,
			cursor.PublishedAt,
			cursor.VideoID,
		)
	}

	err := query.
		Order("hot_score DESC").
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

// ListFollowingPage 使用推拉混合模式读取关注流：普通作者读 inbox，大 V 作者按关注关系拉取。
func (r *Repository) ListFollowingPage(ctx context.Context, viewerID int64, cursor *domainfeed.TimelineCursor, limit int) ([]*domainfeed.FeedPageItem, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("feed repository is not initialized")
	}
	if viewerID <= 0 {
		return nil, domainfeed.ErrViewerRequired
	}
	if limit <= 0 {
		return nil, domainfeed.ErrInvalidLimit
	}
	if limit > domainfeed.MaxLimit+1 {
		limit = domainfeed.MaxLimit + 1
	}
	if cursor != nil && (cursor.PublishedAt.IsZero() || cursor.VideoID <= 0) {
		return nil, domainfeed.ErrInvalidCursor
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var models []domainfeed.FeedPageItem
	query := `
SELECT merged.video_id, merged.published_at
FROM (
	SELECT v.id AS video_id, v.published_at AS published_at
	FROM feed_inbox AS fi
	JOIN videos AS v ON v.id = fi.video_id
	JOIN user_follow AS f ON f.user_id = fi.user_id AND f.target_user_id = v.author_id
	WHERE fi.user_id = ? AND f.status = ? AND v.status = ? AND v.published_at IS NOT NULL
	UNION
	SELECT v.id AS video_id, v.published_at AS published_at
	FROM videos AS v
	JOIN user_follow AS f ON f.target_user_id = v.author_id
	JOIN user_relation_stat AS rs ON rs.user_id = v.author_id
	WHERE f.user_id = ? AND f.status = ? AND rs.follower_count >= ? AND v.status = ? AND v.published_at IS NOT NULL
) AS merged`
	args := []any{
		viewerID,
		domainrelation.FollowStatusActive,
		domainvideo.StatusPublished,
		viewerID,
		domainrelation.FollowStatusActive,
		domainfeed.BigCreatorFollowerThreshold,
		domainvideo.StatusPublished,
	}

	if cursor != nil {
		query += `
WHERE (merged.published_at < ? OR (merged.published_at = ? AND merged.video_id < ?))`
		args = append(args, cursor.PublishedAt, cursor.PublishedAt, cursor.VideoID)
	}

	query += `
ORDER BY merged.published_at DESC, merged.video_id DESC
LIMIT ?`
	args = append(args, limit)

	err := r.db.WithContext(ctx).
		Raw(query, args...).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}
	return feedPageItemsFromModels(models), nil
}

// BatchGetFeedCards 关联 users 表批量读取视频和作者展示字段，供缓存缺失时回源。
func (r *Repository) BatchGetFeedCards(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedCard, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("feed repository is not initialized")
	}
	cards := map[int64]*domainfeed.FeedCard{}
	if len(videoIDs) == 0 {
		return cards, nil
	}

	var models []domainfeed.FeedCard
	err := r.db.WithContext(ctx).
		Table("videos AS v").
		Select("v.id AS video_id, v.author_id, a.nickname AS author_nickname, a.avatar_url AS author_avatar_url, v.title, v.description, v.media_url, v.cover_url").
		Joins("LEFT JOIN users AS a ON a.id = v.author_id").
		Where("v.id IN ? AND v.status = ? AND v.published_at IS NOT NULL", videoIDs, domainvideo.StatusPublished).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}
	for index := range models {
		cards[models[index].VideoID] = &models[index]
	}
	return cards, nil
}

// BatchGetFeedStats 批量读取互动计数，缺失统计记录时按 0 处理。
func (r *Repository) BatchGetFeedStats(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("feed repository is not initialized")
	}
	stats := map[int64]*domainfeed.FeedStat{}
	if len(videoIDs) == 0 {
		return stats, nil
	}
	for _, videoID := range videoIDs {
		stats[videoID] = &domainfeed.FeedStat{VideoID: videoID}
	}

	var models []domainfeed.FeedStat
	err := r.db.WithContext(ctx).
		Table("video_stat").
		Select("video_id, like_count, comment_count, favorite_count").
		Where("video_id IN ?", videoIDs).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}
	for index := range models {
		stats[models[index].VideoID] = &models[index]
	}
	return stats, nil
}

func (r *Repository) basePageQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("videos AS v").
		Select("v.id AS video_id, v.published_at").
		Where("v.status = ? AND v.published_at IS NOT NULL", domainvideo.StatusPublished)
}

func (r *Repository) baseHotPageQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("videos AS v").
		Select("v.id AS video_id, ("+hotScoreExpression+") AS hot_score, v.published_at").
		Joins("LEFT JOIN video_stat AS vs ON vs.video_id = v.id").
		Where("v.status = ? AND v.published_at IS NOT NULL", domainvideo.StatusPublished)
}

// feedPageItemsFromModels 将查询结果转换为指针切片，供领域层分页使用。
func feedPageItemsFromModels(models []domainfeed.FeedPageItem) []*domainfeed.FeedPageItem {
	items := make([]*domainfeed.FeedPageItem, 0, len(models))
	for index := range models {
		items = append(items, &models[index])
	}
	return items
}
