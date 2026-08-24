package config

import (
	"strings"

	"github.com/spf13/viper"
)

// Load 从指定文件路径加载配置
func Load(filename string) (*Config, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return nil, ErrEmptyConfigPath
	}

	v := viper.New()
	v.SetConfigFile(filename)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return nil, ErrReadConfigFailed
	}

	var cfg Config

	if err := v.UnmarshalExact(&cfg); err != nil {
		return nil, ErrUnmarshalConfigFailed
	}
	if err := validateConfig(&cfg); err != nil {
		return nil, ErrValidateConfigFailed
	}
	return &cfg, nil
}
