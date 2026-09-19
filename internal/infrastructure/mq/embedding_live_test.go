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

// TestEmbeddingQueueLive 验证同一发布同时送达两个独立队列，向量消费失败独立重投，非法消息丢弃，取消后清理拓扑。
func TestEmbeddingQueueLive(t *testing.T) {
	url := os.Getenv("VIDEOFEED_TEST_AMQP_URL")
	if url == "" {
		t.Skip("未配置专用 RabbitMQ 测试连接")
	}
	name := fmt.Sprintf("videofeed.test.embedding.%d", time.Now().UnixNano())
	cfg := config.RabbitMQConfig{URL: url, InteractionExchange: name + ".interaction", ActionChangedQueue: name + ".actions", ActionChangedRouting: "action", VideoExchange: name + ".video", VideoPublishedQueue: name + ".fanout", VideoEmbeddingQueue: name + ".embedding", VideoPublishedRouting: "published", ExposureExchange: name + ".exposure", ViewEventRecordedQueue: name + ".views", ViewEventRecordedRouting: "view"}
	broker, err := NewRabbitMQ(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	defer func() {
		for _, queue := range []string{cfg.ActionChangedQueue, cfg.VideoPublishedQueue, cfg.VideoEmbeddingQueue, cfg.ViewEventRecordedQueue} {
			if _, err := broker.publish.QueueDelete(queue, false, false, false); err != nil {
				t.Error(err)
			}
		}
		for _, exchange := range []string{cfg.InteractionExchange, cfg.VideoExchange, cfg.ExposureExchange} {
			if err := broker.publish.ExchangeDelete(exchange, false, false); err != nil {
				t.Error(err)
			}
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	fanout := make(chan *applicationvideo.PublishedEvent, 4)
	embedding := make(chan *applicationvideo.PublishedEvent, 4)
	var attempts atomic.Int32
	if err := broker.ConsumeVideoPublished(ctx, func(_ context.Context, event *applicationvideo.PublishedEvent) error { fanout <- event; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := broker.ConsumeVideoPublishedForEmbedding(ctx, func(_ context.Context, event *applicationvideo.PublishedEvent) error {
		if attempts.Add(1) == 1 {
			return errors.New("retry embedding once")
		}
		embedding <- event
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := broker.publish.PublishWithContext(ctx, cfg.VideoExchange, cfg.VideoPublishedRouting, false, false, amqp.Publishing{Body: []byte("not-json")}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		event := &applicationvideo.PublishedEvent{EventID: fmt.Sprint(i), VideoID: 42, Title: "title", Description: "desc", PublishedAt: time.Now().UTC()}
		if err := broker.PublishVideoPublished(ctx, event); err != nil {
			t.Fatal(err)
		}
		for _, messages := range []chan *applicationvideo.PublishedEvent{fanout, embedding} {
			select {
			case got := <-messages:
				if got.EventID != event.EventID || got.Title != event.Title || got.Description != event.Description || !got.PublishedAt.Equal(event.PublishedAt) {
					t.Fatal(got)
				}
			case <-ctx.Done():
				t.Fatal("独立队列未收到发布事件")
			}
		}
	}
	if attempts.Load() != 4 {
		t.Fatalf("attempts=%d", attempts.Load())
	}
	cancel()
	deadline := time.Now().Add(5 * time.Second)
	for _, queue := range []string{cfg.VideoPublishedQueue, cfg.VideoEmbeddingQueue} {
		for {
			state, err := broker.publish.QueueInspect(queue)
			if err != nil {
				t.Fatal(err)
			}
			if state.Consumers == 0 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("消费者取消超时")
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	if err := broker.consume.Qos(1, 0, false); err != nil {
		t.Fatal(err)
	}
	for _, queue := range []string{cfg.VideoPublishedQueue, cfg.VideoEmbeddingQueue} {
		if _, ok, err := broker.publish.Get(queue, true); err != nil || ok {
			t.Fatalf("queue=%s remaining=%v err=%v", queue, ok, err)
		}
	}
}
