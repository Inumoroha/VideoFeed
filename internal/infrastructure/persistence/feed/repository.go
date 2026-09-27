package feed

import (
	"context"
	"errors"
	"fmt"

	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	domaininteraction "github.com/inumoroha/video-feed/internal/domain/interaction"
	domainrelation "github.com/inumoroha/video-feed/internal/domain/relation"
	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"
	"gorm.io/gorm"
)

const hotScoreExpression = "COALESCE(vs.like_count, 0) * 3 + COALESCE(vs.comment_count, 0) * 5 + COALESCE(vs.favorite_count, 0) * 4"

// Repository 使用 GORM 持久化信息流数据。
type Repository struct {
	db *gorm.DB
}

// viewerActionStateModel 接收当前用户互动状态查询的视频编号和动作类型。
type viewerActionStateModel struct {
	VideoID    int64
	ActionType string
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

// ListFollowingPage 按有效关注关系读取公开视频，供 Redis 索引缺失时回退。
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
	query := r.db.WithContext(ctx).
		Table("videos AS v").
		Select("v.id AS video_id, v.author_id, v.published_at").
		Joins("JOIN user_follow AS f ON f.target_user_id = v.author_id").
		Where("f.user_id = ? AND f.status = ? AND v.status = ? AND v.published_at IS NOT NULL", viewerID, domainrelation.FollowStatusActive, domainvideo.StatusPublished)

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

// ListFollowingPullAuthorIDs 查询当前用户关注的大作者编号。
func (r *Repository) ListFollowingPullAuthorIDs(ctx context.Context, viewerID int64) ([]int64, error) {
	var authorIDs []int64
	err := r.db.WithContext(ctx).
		Table("user_follow AS f").
		Select("f.target_user_id").
		Joins("JOIN user_relation_stat AS rs ON rs.user_id = f.target_user_id").
		Where("f.user_id = ? AND f.status = ? AND rs.follower_count >= ?", viewerID, domainrelation.FollowStatusActive, domainfeed.BigCreatorFollowerThreshold).
		Order("f.target_user_id ASC").
		Scan(&authorIDs).
		Error
	return authorIDs, err
}

// CountFollowers 读取作者粉丝数，缺失统计时按零处理。
func (r *Repository) CountFollowers(ctx context.Context, authorID int64) (int, error) {
	var stat struct{ FollowerCount int }
	err := r.db.WithContext(ctx).
		Table("user_relation_stat").
		Select("follower_count").
		Where("user_id = ?", authorID).
		Take(&stat).
		Error
	if err == gorm.ErrRecordNotFound {
		return 0, nil
	}
	return stat.FollowerCount, err
}

// ListFollowerIDs 按用户编号升序分页读取作者的有效粉丝。
func (r *Repository) ListFollowerIDs(ctx context.Context, authorID int64, cursor int64, limit int) ([]int64, error) {
	var followerIDs []int64
	query := r.db.WithContext(ctx).
		Table("user_follow").
		Select("user_id").
		Where("target_user_id = ? AND status = ?", authorID, domainrelation.FollowStatusActive)
	if cursor > 0 {
		query = query.Where("user_id > ?", cursor)
	}
	err := query.
		Order("user_id ASC").
		Limit(limit).
		Scan(&followerIDs).
		Error
	return followerIDs, err
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

// BatchGetViewerActionStates 批量读取当前用户对视频的点赞和收藏状态，未命中按未互动处理。
func (r *Repository) BatchGetViewerActionStates(ctx context.Context, viewerID int64, videoIDs []int64) (map[int64]*domainfeed.ViewerActionState, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("feed repository is not initialized")
	}
	states := map[int64]*domainfeed.ViewerActionState{}
	if viewerID <= 0 || len(videoIDs) == 0 {
		return states, nil
	}
	for _, videoID := range videoIDs {
		states[videoID] = &domainfeed.ViewerActionState{VideoID: videoID}
	}

	var models []viewerActionStateModel
	err := r.db.WithContext(ctx).
		Table("interaction_action").
		Select("video_id, action_type").
		Where("user_id = ? AND video_id IN ? AND status = ?", viewerID, videoIDs, domaininteraction.ActionStatusActive).
		Where("action_type IN ?", []string{domaininteraction.ActionTypeLike, domaininteraction.ActionTypeFavorite}).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}
	for _, model := range models {
		state := states[model.VideoID]
		if state == nil {
			state = &domainfeed.ViewerActionState{VideoID: model.VideoID}
			states[model.VideoID] = state
		}
		switch model.ActionType {
		case domaininteraction.ActionTypeLike:
			state.Liked = true
		case domaininteraction.ActionTypeFavorite:
			state.Favorited = true
		}
	}
	return states, nil
}

// basePageQuery 创建携带作者编号的公开视频分页查询。
func (r *Repository) basePageQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("videos AS v").
		Select("v.id AS video_id, v.author_id, v.published_at").
		Where("v.status = ? AND v.published_at IS NOT NULL", domainvideo.StatusPublished)
}

// baseHotPageQuery 创建携带作者编号和互动热度的公开视频查询。
func (r *Repository) baseHotPageQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("videos AS v").
		Select("v.id AS video_id, v.author_id, ("+hotScoreExpression+") AS hot_score, v.published_at").
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

// ListFollowingAuthorIDs 查询全部有效关注作者，用于过滤已取关的缓存条目。
func (r *Repository) ListFollowingAuthorIDs(ctx context.Context, viewerID int64) ([]int64, error) {
	var ids []int64
	err := r.db.WithContext(ctx).Table("user_follow").Select("target_user_id").Where("user_id = ? AND status = ?", viewerID, domainrelation.FollowStatusActive).Order("target_user_id ASC").Scan(&ids).Error
	return ids, err
}

// ValidateFollowingIndex 批量核对索引视频的发布状态和有效关注关系，避免预热卡片泄漏已删除视频。
func (r *Repository) ValidateFollowingIndex(ctx context.Context, viewerID int64, items []*domainfeed.FeedPageItem) (bool, error) {
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		if item != nil {
			ids = append(ids, item.VideoID)
		}
	}
	if len(ids) == 0 {
		return true, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Table("videos AS v").Joins("JOIN user_follow AS f ON f.target_user_id = v.author_id").Where("v.id IN ? AND v.status = ? AND v.published_at IS NOT NULL AND f.user_id = ? AND f.status = ?", ids, domainvideo.StatusPublished, viewerID, domainrelation.FollowStatusActive).Count(&count).Error
	return count == int64(len(items)), err
}
