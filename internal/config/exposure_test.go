package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExposureQueueEnvironment 验证新增消息拓扑字段在旧配置文件中缺省时仍可由环境变量绑定。
func TestExposureQueueEnvironment(t *testing.T) {
	content, err := os.ReadFile("../../configs/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_URL", "postgres://test@localhost/test")
	t.Setenv("JWT_SECRET", "test-only-secret-for-config-validation-2026")
	lines := strings.Split(string(content), "\n")
	kept := []string{}
	for _, line := range lines {
		if !strings.Contains(line, "exposure_exchange:") && !strings.Contains(line, "view_event_recorded_queue:") && !strings.Contains(line, "view_event_recorded_routing:") {
			kept = append(kept, line)
		}
	}
	content = []byte(strings.Join(kept, "\n"))
	dir := t.TempDir()
	filename := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(filename, content, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RABBITMQ_EXPOSURE_EXCHANGE", "test.video")
	t.Setenv("RABBITMQ_VIEW_EVENT_RECORDED_QUEUE", "test.published")
	t.Setenv("RABBITMQ_VIEW_EVENT_RECORDED_ROUTING", "test.routing")
	cfg, err := Load(filename)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RabbitMQ.ExposureExchange != "test.video" || cfg.RabbitMQ.ViewEventRecordedQueue != "test.published" || cfg.RabbitMQ.ViewEventRecordedRouting != "test.routing" {
		t.Fatal("消息拓扑环境变量未生效")
	}
}
