package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEmbeddingQueueEnvironment 验证旧配置缺少向量队列字段时仍可用环境变量覆盖。
func TestEmbeddingQueueEnvironment(t *testing.T) {
	content, err := os.ReadFile("../../configs/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	kept := []string{}
	for _, line := range strings.Split(string(content), "\n") {
		if !strings.Contains(line, "video_embedding_queue:") {
			kept = append(kept, line)
		}
	}
	filename := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(filename, []byte(strings.Join(kept, "\n")), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_URL", "postgres://test@localhost/test")
	t.Setenv("JWT_SECRET", "test-only-secret-for-embedding-config-2026")
	t.Setenv("RABBITMQ_VIDEO_EMBEDDING_QUEUE", "custom.embedding")
	cfg, err := Load(filename)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RabbitMQ.VideoEmbeddingQueue != "custom.embedding" {
		t.Fatal("向量队列环境变量未生效")
	}
}
