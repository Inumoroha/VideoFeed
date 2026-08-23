package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Load 从指定文件路径加载配置
func Load(filename string) (*Config, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return nil, fmt.Errorf("config file path is empty")
	}

	v := viper.New()
	v.SetConfigFile(filename)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config

	if err := v.UnmarshalExact(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config into struct: %w", err)
	}
	if err := validateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}
	return &cfg, nil
}
