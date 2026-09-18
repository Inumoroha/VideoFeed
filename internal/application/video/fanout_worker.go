package video

import (
	"context"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	"time"
)

const defaultFollowerBatchSize = 500

type PublishedEventConsumer interface {
	ConsumeVideoPublished(ctx context.Context, handler func(context.Context, *PublishedEvent) error) error
}

type FanoutRepository interface {
	CountFollowers(ctx context.Context, authorID int64) (int, error)
	ListFollowerIDs(ctx context.Context, authorID int64, cursor int64, limit int) ([]int64, error)
	BatchGetFeedCards(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedCard, error)
	BatchGetFeedStats(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error)
}

type FollowingIndexCache interface {
	AddInboxItems(ctx context.Context, authorID int64, userIDs []int64, item *domainfeed.FeedPageItem, maxLen int64) error
	AddAuthorOutboxItem(ctx context.Context, authorID int64, item *domainfeed.FeedPageItem, maxLen int64) error
}

type FeedPreheater interface {
	PreheatFeedVideo(ctx context.Context, videoID int64, ttl time.Duration) error
}

type FeedCacheWriter interface {
	SetCards(ctx context.Context, cards map[int64]*domainfeed.FeedCard, ttl time.Duration) error
	SetStats(ctx context.Context, stats map[int64]*domainfeed.FeedStat, ttl time.Duration) error
}

// FanoutWorker 保存粉丝分页、索引写入和预热依赖及容量参数。
type FanoutWorker struct {
	repo         FanoutRepository
	consumer     PublishedEventConsumer
	index        FollowingIndexCache
	preheater    FeedPreheater
	batchSize    int
	inboxMaxLen  int64
	outboxMaxLen int64
	preheatTTL   time.Duration
}

type FanoutWorkerOption func(*FanoutWorker)

// NewFanoutWorker 创建分发任务，默认每批五百粉丝并设置索引容量和预热有效期。
func NewFanoutWorker(repo FanoutRepository, consumer PublishedEventConsumer, index FollowingIndexCache, preheater FeedPreheater, options ...FanoutWorkerOption) *FanoutWorker {
	worker := &FanoutWorker{
		repo:         repo,
		consumer:     consumer,
		index:        index,
		preheater:    preheater,
		batchSize:    defaultFollowerBatchSize,
		inboxMaxLen:  1000,
		outboxMaxLen: 500,
		preheatTTL:   15 * time.Minute,
	}
	for _, option := range options {
		option(worker)
	}
	return worker
}

// FeedPreheaterAdapter 连接数据库卡片与统计读取和缓存写入。
type FeedPreheaterAdapter struct {
	source FanoutRepository
	cache  FeedCacheWriter
}

// NewFeedPreheater 创建使用指定数据源和缓存的预热适配器。
func NewFeedPreheater(source FanoutRepository, cache FeedCacheWriter) *FeedPreheaterAdapter {
	return &FeedPreheaterAdapter{source: source, cache: cache}
}

// PreheatFeedVideo 读取并缓存视频卡片和统计，任一步失败即返回错误供消息重试。
func (p *FeedPreheaterAdapter) PreheatFeedVideo(ctx context.Context, videoID int64, ttl time.Duration) error {
	if p == nil || p.source == nil || p.cache == nil || videoID <= 0 {
		return nil
	}
	cards, err := p.source.BatchGetFeedCards(ctx, []int64{videoID})
	if err != nil {
		return err
	}
	if len(cards) > 0 {
		if err := p.cache.SetCards(ctx, cards, ttl); err != nil {
			return err
		}
	}
	stats, err := p.source.BatchGetFeedStats(ctx, []int64{videoID})
	if err != nil {
		return err
	}
	if len(stats) > 0 {
		return p.cache.SetStats(ctx, stats, 15*time.Second)
	}
	return nil
}

// WithFanoutBatchSize 设置正数粉丝分页大小，非正数保留默认值。
func WithFanoutBatchSize(size int) FanoutWorkerOption {
	return func(w *FanoutWorker) {
		if size > 0 {
			w.batchSize = size
		}
	}
}

// Start 注册发布事件消费者，未配置消费者时不启动任务。
func (w *FanoutWorker) Start(ctx context.Context) error {
	if w == nil || w.consumer == nil {
		return nil
	}
	return w.consumer.ConsumeVideoPublished(ctx, w.HandleVideoPublished)
}

// HandleVideoPublished 先预热再按粉丝阈值分发，大作者写发件箱，普通作者分页写收件箱；失败交由消费者重试。
func (w *FanoutWorker) HandleVideoPublished(ctx context.Context, event *PublishedEvent) error {
	if event == nil || event.VideoID <= 0 || event.AuthorID <= 0 {
		return nil
	}
	if err := w.preheat(ctx, event); err != nil {
		return err
	}
	if w.repo == nil || w.index == nil {
		return nil
	}

	item := &domainfeed.FeedPageItem{
		VideoID:     event.VideoID,
		AuthorID:    event.AuthorID,
		PublishedAt: event.PublishedAt,
	}

	followerCount, err := w.repo.CountFollowers(ctx, event.AuthorID)
	if err != nil {
		return err
	}
	if followerCount >= domainfeed.BigCreatorFollowerThreshold {
		return w.index.AddAuthorOutboxItem(ctx, event.AuthorID, item, w.outboxMaxLen)
	}

	cursor := int64(0)
	for {
		followerIDs, err := w.repo.ListFollowerIDs(ctx, event.AuthorID, cursor, w.batchSize)
		if err != nil {
			return err
		}
		if len(followerIDs) == 0 {
			return nil
		}
		if err := w.index.AddInboxItems(ctx, event.AuthorID, followerIDs, item, w.inboxMaxLen); err != nil {
			return err
		}
		cursor = followerIDs[len(followerIDs)-1]
		if len(followerIDs) < w.batchSize {
			return nil
		}
	}
}

// preheat 在配置预热器时以指定有效期预热视频缓存。
func (w *FanoutWorker) preheat(ctx context.Context, event *PublishedEvent) error {
	if w.preheater == nil {
		return nil
	}
	return w.preheater.PreheatFeedVideo(ctx, event.VideoID, w.preheatTTL)
}
