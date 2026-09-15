package mq

import (
	"testing"

	"github.com/inumoroha/video-feed/internal/config"
)

// TestNormalizeConfig 验证 RabbitMQ 拓扑配置缺省值会被补齐。
func TestNormalizeConfig(t *testing.T) {
	c := normalizeConfig(config.RabbitMQConfig{})
	if c.InteractionExchange == "" || c.ActionChangedQueue == "" || c.ActionChangedRouting == "" {
		t.Fatalf("默认 RabbitMQ 配置未补齐: %+v", c)
	}
}
