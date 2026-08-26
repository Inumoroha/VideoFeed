package router

import (
	"testing"

	"github.com/inumoroha/video-feed/internal/config"
)

// TestSetupRejectsNilConfig 验证路由初始化会拒绝空应用配置。
func TestSetupRejectsNilConfig(t *testing.T) {
	engine, err := Setup(nil, nil)
	if err == nil {
		t.Fatal("Setup() error = nil, want non-nil")
	}
	if engine != nil {
		t.Fatal("Setup() engine is not nil on error")
	}
}

// TestSetupRejectsNilDatabase 验证路由初始化会拒绝空 GORM 数据库实例。
func TestSetupRejectsNilDatabase(t *testing.T) {
	engine, err := Setup(&config.Config{}, nil)
	if err == nil {
		t.Fatal("Setup() error = nil, want non-nil")
	}
	if engine != nil {
		t.Fatal("Setup() engine is not nil on error")
	}
}
