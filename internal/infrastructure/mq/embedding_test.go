package mq

import (
	"github.com/inumoroha/video-feed/internal/config"
	"testing"
)

// TestEmbeddingQueueDefaults 验证向量队列默认名称、去空白以及独立于既有分发队列的配置。
func TestEmbeddingQueueDefaults(t *testing.T) {
	for _, value := range []string{"", "  "} {
		cfg := normalizeConfig(config.RabbitMQConfig{VideoEmbeddingQueue: value})
		if cfg.VideoEmbeddingQueue != "videofeed.video.embedding" || cfg.VideoEmbeddingQueue == cfg.VideoPublishedQueue {
			t.Fatal(cfg)
		}
	}
	cfg := normalizeConfig(config.RabbitMQConfig{VideoEmbeddingQueue: " custom.embedding "})
	if cfg.VideoEmbeddingQueue != "custom.embedding" {
		t.Fatal(cfg)
	}
}
