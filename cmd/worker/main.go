package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"strings"
	"syscall"

	applicationembedding "github.com/inumoroha/video-feed/internal/application/embedding"
	applicationinteraction "github.com/inumoroha/video-feed/internal/application/interaction"
	applicationvideo "github.com/inumoroha/video-feed/internal/application/video"
	"github.com/inumoroha/video-feed/internal/config"
	"github.com/inumoroha/video-feed/internal/database"
	feedcache "github.com/inumoroha/video-feed/internal/infrastructure/cache/feed"
	infraredis "github.com/inumoroha/video-feed/internal/infrastructure/cache/redis"
	inframetrics "github.com/inumoroha/video-feed/internal/infrastructure/metrics"
	inframq "github.com/inumoroha/video-feed/internal/infrastructure/mq"
	embedding "github.com/inumoroha/video-feed/internal/infrastructure/persistence/embedding"
	feed "github.com/inumoroha/video-feed/internal/infrastructure/persistence/feed"
	interaction "github.com/inumoroha/video-feed/internal/infrastructure/persistence/interaction"
	"github.com/inumoroha/video-feed/internal/infrastructure/persistence/migration"
	"gorm.io/gorm"
)

const configPath = "./configs/config.yaml"

// metricsAddr 定义 worker 指标服务的监听地址，供 Prometheus 抓取。
const metricsAddr = ":9091"

// main 加载共享配置并监听终止信号；运行失败时在资源释放完成后以非零状态退出。
func main() {
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	// 独立指标服务暴露 worker 指标，随机终止信号到达时优雅关闭。
	go func() {
		if err := inframetrics.RunServer(ctx, metricsAddr); err != nil {
			log.Printf("metrics server failed: %v", err)
		}
	}()
	if err := run(ctx, cfg); err != nil {
		log.Fatalf("worker failed: %v", err)
	}
}

// run 校验依赖、迁移数据库并启动三个消费者，取消或启动失败时依次关闭消息、缓存和数据库连接。
func run(ctx context.Context, cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("application config is nil")
	}
	if strings.TrimSpace(cfg.RabbitMQ.URL) == "" {
		return fmt.Errorf("rabbitmq url is required for worker")
	}
	if strings.TrimSpace(cfg.Redis.Addr) == "" {
		return fmt.Errorf("redis addr is required for worker")
	}
	db, err := database.New(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("init database: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	if err := migration.AutoMigrate(db.WithContext(ctx)); err != nil {
		return err
	}
	client := infraredis.NewRedisClient(cfg.Redis)
	defer client.Close()
	broker, err := inframq.NewRabbitMQ(cfg.RabbitMQ)
	if err != nil {
		return fmt.Errorf("init rabbitmq: %w", err)
	}
	defer broker.Close()
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := startWorkers(workerCtx, db, broker, feedcache.NewFeedCache(client)); err != nil {
		return fmt.Errorf("start workers: %w", err)
	}
	log.Println("videofeed worker is running")
	<-ctx.Done()
	log.Println("videofeed worker stopped")
	return nil
}

// eventConsumer 汇总三个独立消息队列的订阅能力，便于验证启动顺序和失败短路。
type eventConsumer interface {
	applicationinteraction.ActionEventConsumer
	applicationvideo.PublishedEventConsumer
	applicationembedding.PublishedEventConsumer
}

// startWorkers 按互动、分发和向量顺序注册消费者，任一步失败立即返回；缓存和连接由调用方管理。
func startWorkers(ctx context.Context, db *gorm.DB, broker eventConsumer, cache *feedcache.FeedCache) error {
	if err := applicationinteraction.NewActionWorker(interaction.New(db), broker).Start(ctx); err != nil {
		return err
	}
	repo := feed.New(db)
	preheater := applicationvideo.NewFeedPreheater(repo, cache)
	if err := applicationvideo.NewFanoutWorker(repo, broker, cache, preheater).Start(ctx); err != nil {
		return err
	}
	service := applicationembedding.NewService(embedding.New(db), nil)
	return applicationembedding.NewVideoEmbeddingWorker(service, broker).Start(ctx)
}
