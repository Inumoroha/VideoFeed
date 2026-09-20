package main

import (
	"context"
	"errors"
	interaction "github.com/inumoroha/video-feed/internal/application/interaction"
	video "github.com/inumoroha/video-feed/internal/application/video"
	"github.com/inumoroha/video-feed/internal/config"
	"reflect"
	"testing"
)

// testConsumer 记录订阅顺序、上下文和指定阶段的启动错误。
type testConsumer struct {
	calls    []string
	contexts []context.Context
	fail     string
}

// record 记录启动阶段并按配置返回错误。
func (b *testConsumer) record(ctx context.Context, name string) error {
	b.calls = append(b.calls, name)
	b.contexts = append(b.contexts, ctx)
	if name == b.fail {
		return errors.New(name)
	}
	return nil
}

// ConsumeActionChanged 记录互动消费者注册。
func (b *testConsumer) ConsumeActionChanged(ctx context.Context, h func(context.Context, *interaction.ActionChangedEvent) error) error {
	return b.record(ctx, "action")
}

// ConsumeVideoPublished 记录关注流分发消费者注册。
func (b *testConsumer) ConsumeVideoPublished(ctx context.Context, h func(context.Context, *video.PublishedEvent) error) error {
	return b.record(ctx, "fanout")
}

// ConsumeVideoPublishedForEmbedding 记录独立向量消费者注册。
func (b *testConsumer) ConsumeVideoPublishedForEmbedding(ctx context.Context, h func(context.Context, *video.PublishedEvent) error) error {
	return b.record(ctx, "embedding")
}

// TestStartWorkers 验证三类队列顺序、失败短路、无隐式重试及取消传递。
func TestStartWorkers(t *testing.T) {
	for _, stage := range []string{"", "action", "fanout", "embedding"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			broker := &testConsumer{fail: stage}
			err := startWorkers(ctx, nil, broker, nil)
			expected := []string{"action", "fanout", "embedding"}
			if stage == "action" {
				expected = expected[:1]
			}
			if stage == "fanout" {
				expected = expected[:2]
			}
			if !reflect.DeepEqual(expected, broker.calls) || (err != nil) != (stage != "") {
				t.Fatal(broker.calls, err)
			}
			cancel()
			for _, received := range broker.contexts {
				if received.Err() != context.Canceled {
					t.Fatal("未传播取消")
				}
			}
		})
	}
}

// TestRunRequiresDependencies 验证缺失配置、消息地址或缓存地址在连接数据库前失败。
func TestRunRequiresDependencies(t *testing.T) {
	for _, cfg := range []*config.Config{nil, {}, {RabbitMQ: config.RabbitMQConfig{URL: "amqp://example"}}} {
		if err := run(context.Background(), cfg); err == nil {
			t.Fatal("缺失依赖未失败")
		}
	}
}
