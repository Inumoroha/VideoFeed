package mq

import (
	"testing"

	"github.com/inumoroha/video-feed/internal/config"
)

// TestNormalizeConfig 验证 RabbitMQ 拓扑配置缺省值会被补齐。
func TestNormalizeConfig(t *testing.T) {
	c := normalizeConfig(config.RabbitMQConfig{})
	if c.InteractionExchange == "" || c.ActionChangedQueue == "" || c.ActionChangedRouting == "" || c.VideoExchange != "videofeed.video" || c.VideoPublishedQueue != "videofeed.video.published" || c.VideoPublishedRouting != "video.published" {
		t.Fatalf("默认 RabbitMQ 配置未补齐: %+v", c)
	}
}

// TestVideoTopologyOverrides 验证自定义视频拓扑保留且空白配置会补齐默认值。
func TestVideoTopologyOverrides(t *testing.T) {
	c := normalizeConfig(config.RabbitMQConfig{VideoExchange: "custom", VideoPublishedQueue: "queue", VideoPublishedRouting: "routing"})
	if c.VideoExchange != "custom" || c.VideoPublishedQueue != "queue" || c.VideoPublishedRouting != "routing" {
		t.Fatal("custom topology lost")
	}
	c = normalizeConfig(config.RabbitMQConfig{VideoExchange: " ", VideoPublishedQueue: " ", VideoPublishedRouting: " "})
	if c.VideoExchange != defaultVideoExchange || c.VideoPublishedQueue != defaultVideoPublishedQueue || c.VideoPublishedRouting != defaultVideoPublishedRouting {
		t.Fatal("blank defaults missing")
	}
}
