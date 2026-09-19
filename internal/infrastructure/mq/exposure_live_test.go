package mq

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	applicationexposure "github.com/inumoroha/video-feed/internal/application/exposure"
	"github.com/inumoroha/video-feed/internal/config"
	amqp "github.com/rabbitmq/amqp091-go"
)

// TestExposureQueueLive 在专用消息服务创建隔离拓扑，验证画像消息属性、重投与确认、取消消费、初始化失败和连接关闭，并清理资源。
func TestExposureQueueLive(t *testing.T) {
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
	original := &applicationexposure.ViewEventRecordedEvent{EventID: "attributes", ViewEventID: 1}
	if err := broker.PublishViewEventRecorded(ctx, original); err != nil {
		t.Fatal(err)
	}
	// 发布与 Get 共用发布通道，服务端按通道顺序执行。
	delivered, ok, err := broker.publish.Get(cfg.ViewEventRecordedQueue, false)
	if err != nil || !ok || delivered.MessageId != "attributes" || delivered.DeliveryMode != amqp.Persistent || delivered.ContentType != "application/json" || delivered.Timestamp.IsZero() {
		t.Fatalf("消息属性错误：ok=%v err=%v", ok, err)
	}
	if err := delivered.Ack(false); err != nil {
		t.Fatal(err)
	}
	var attempts atomic.Int32
	done := make(chan *applicationexposure.ViewEventRecordedEvent, 1)
	if err := broker.ConsumeViewEventRecorded(ctx, func(_ context.Context, event *applicationexposure.ViewEventRecordedEvent) error {
		if attempts.Add(1) == 1 {
			return errors.New("retry once")
		}
		done <- event
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := broker.publish.PublishWithContext(ctx, cfg.ExposureExchange, cfg.ViewEventRecordedRouting, false, false, amqp.Publishing{Body: []byte("invalid json")}); err != nil {
		t.Fatal(err)
	}
	event := &applicationexposure.ViewEventRecordedEvent{EventID: "roundtrip", ViewEventID: 3, VideoID: 99, UserID: 7, Scene: "hot", EventType: "complete", Completed: true, RecordedAt: time.Now().UTC(), OccurredAt: time.Now().UTC()}
	if err := broker.PublishViewEventRecorded(ctx, event); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if got.EventID != event.EventID || got.VideoID != 99 || !got.RecordedAt.Equal(event.RecordedAt) {
			t.Fatalf("event=%+v", got)
		}
	case <-ctx.Done():
		t.Fatal("画像消息未按时送达")
	}
	if attempts.Load() != 2 {
		t.Fatalf("attempts=%d", attempts.Load())
	}
	q, err := broker.publish.QueueInspect(cfg.ViewEventRecordedQueue)
	if err != nil || q.Messages != 0 {
		t.Fatalf("pending=%d err=%v", q.Messages, err)
	}
	if err := broker.PublishViewEventRecorded(ctx, nil); err != nil {
		t.Fatal(err)
	}
	cancel()
	deadline := time.Now().Add(5 * time.Second)
	for {
		queue, err := broker.publish.QueueInspect(cfg.ViewEventRecordedQueue)
		if err != nil {
			t.Fatal(err)
		}
		if queue.Consumers == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("取消上下文后消费者未退出")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// 消费通道上的同步请求作为屏障，确保此前确认已到达服务端。
	if err := broker.consume.Qos(1, 0, false); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := broker.publish.Get(cfg.ViewEventRecordedQueue, true); err != nil || ok {
		t.Fatalf("成功确认或毒消息拒绝后队列不为空：ok=%v err=%v", ok, err)
	}
	closing, err := NewRabbitMQ(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := closing.Close(); err != nil {
		t.Fatal(err)
	}
	if !closing.conn.IsClosed() || !closing.publish.IsClosed() || !closing.consume.IsClosed() {
		t.Fatal("关闭后仍有活动连接或通道")
	}
	if err := closing.PublishViewEventRecorded(context.Background(), event); err == nil {
		t.Fatal("关闭后的发布应返回错误")
	}
	if err := closing.ConsumeViewEventRecorded(context.Background(), func(context.Context, *applicationexposure.ViewEventRecordedEvent) error { return nil }); err == nil {
		t.Fatal("关闭后的消费应返回错误")
	}
	wrongExchange := name + ".wrong-type"
	if err := broker.publish.ExchangeDeclare(wrongExchange, "direct", true, false, false, false, nil); err != nil {
		t.Fatal(err)
	}
	defer broker.publish.ExchangeDelete(wrongExchange, false, false)
	badConfig := cfg
	badConfig.ExposureExchange = wrongExchange
	if unexpected, err := NewRabbitMQ(badConfig); err == nil {
		_ = unexpected.Close()
		t.Fatal("交换机类型冲突应导致初始化失败")
	}
}
