package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestVideoQueueEnvironment 验证新增消息拓扑字段在旧配置文件中缺省时仍可由环境变量绑定。
func TestVideoQueueEnvironment(t *testing.T) {
	content, err := os.ReadFile("../../configs/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_URL", "postgres://test@localhost/test")
	t.Setenv("JWT_SECRET", "test-only-secret-for-config-validation-2026")
	lines := strings.Split(string(content), "\n")
	kept := []string{}
	for _, line := range lines {
		if !strings.Contains(line, "video_exchange:") && !strings.Contains(line, "video_published_queue:") && !strings.Contains(line, "video_published_routing:") {
			kept = append(kept, line)
		}
	}
	content = []byte(strings.Join(kept, "\n"))
	dir := t.TempDir()
	filename := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(filename, content, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RABBITMQ_VIDEO_EXCHANGE", "test.video")
	t.Setenv("RABBITMQ_VIDEO_PUBLISHED_QUEUE", "test.published")
	t.Setenv("RABBITMQ_VIDEO_PUBLISHED_ROUTING", "test.routing")
	cfg, err := Load(filename)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RabbitMQ.VideoExchange != "test.video" || cfg.RabbitMQ.VideoPublishedQueue != "test.published" || cfg.RabbitMQ.VideoPublishedRouting != "test.routing" {
		t.Fatal("消息拓扑环境变量未生效")
	}
}
