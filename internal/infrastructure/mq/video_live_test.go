package mq

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	applicationvideo "github.com/inumoroha/video-feed/internal/application/video"
	"github.com/inumoroha/video-feed/internal/config"
	amqp "github.com/rabbitmq/amqp091-go"
)

// TestVideoQueueLive 在显式配置的本地消息服务创建隔离拓扑，验证路由、重试、确认、毒消息拒绝与连接关闭。
func TestVideoQueueLive(t *testing.T) {
	url := os.Getenv("VIDEOFEED_TEST_AMQP_URL")
	if url == "" {
		t.Skip("未配置隔离 RabbitMQ 测试连接")
	}
	name := fmt.Sprintf("videofeed.test.%d", time.Now().UnixNano())
	cfg := config.RabbitMQConfig{ExposureExchange: name + ".exposure", ViewEventRecordedQueue: name + ".views", ViewEventRecordedRouting: "view", URL: url, InteractionExchange: name + ".interaction", ActionChangedQueue: name + ".actions", ActionChangedRouting: "action", VideoExchange: name + ".video", VideoPublishedQueue: name + ".published", VideoPublishedRouting: "published"}
	broker, err := NewRabbitMQ(cfg)
	if err != nil {
		t.Fatal("连接本地测试消息服务失败")
	}
	defer broker.Close()
	defer func() {
		_, _ = broker.publish.QueueDelete(cfg.ViewEventRecordedQueue, false, false, false)
		_ = broker.publish.ExchangeDelete(cfg.ExposureExchange, false, false)
		_, _ = broker.publish.QueueDelete(cfg.VideoPublishedQueue, false, false, false)
		_, _ = broker.publish.QueueDelete(cfg.ActionChangedQueue, false, false, false)
		_ = broker.publish.ExchangeDelete(cfg.VideoExchange, false, false)
		_ = broker.publish.ExchangeDelete(cfg.InteractionExchange, false, false)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var attempts atomic.Int32
	done := make(chan *applicationvideo.PublishedEvent, 1)
	if err := broker.ConsumeVideoPublished(ctx, func(_ context.Context, event *applicationvideo.PublishedEvent) error {
		if attempts.Add(1) == 1 {
			return errors.New("retry once")
		}
		done <- event
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := broker.publish.PublishWithContext(ctx, cfg.VideoExchange, cfg.VideoPublishedRouting, false, false, amqp.Publishing{Body: []byte("invalid json")}); err != nil {
		t.Fatal(err)
	}
	event := &applicationvideo.PublishedEvent{EventID: "roundtrip", VideoID: 99, AuthorID: 7, Title: "test", PublishedAt: time.Now().UTC()}
	if err := broker.PublishVideoPublished(ctx, event); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if got.EventID != event.EventID || got.VideoID != 99 || !got.PublishedAt.Equal(event.PublishedAt) {
			t.Fatalf("event=%+v", got)
		}
	case <-ctx.Done():
		t.Fatal("视频消息未按时送达")
	}
	if attempts.Load() != 2 {
		t.Fatalf("attempts=%d", attempts.Load())
	}
	q, err := broker.publish.QueueInspect(cfg.VideoPublishedQueue)
	if err != nil || q.Messages != 0 {
		t.Fatalf("pending=%d err=%v", q.Messages, err)
	}
	if err := broker.PublishVideoPublished(ctx, nil); err != nil {
		t.Fatal(err)
	}
}
