package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInternalTokenEnvironment 验证旧配置缺少内部令牌字段时仍可绑定环境变量，并允许空令牌关闭创建入口。
func TestInternalTokenEnvironment(t *testing.T) {
	content, err := os.ReadFile("../../configs/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	content = []byte(strings.Split(string(content), "\ninternal:")[0])
	name := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(name, content, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_URL", "postgres://test@localhost/test")
	t.Setenv("JWT_SECRET", "test-only-secret-for-config-validation-2026")
	for _, token := range []string{"test-internal-token", ""} {
		t.Setenv("INTERNAL_TOKEN", token)
		cfg, err := Load(name)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Internal.Token != token {
			t.Fatal("内部令牌环境变量未生效")
		}
	}
}
