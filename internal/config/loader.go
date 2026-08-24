package config

import (
	"path/filepath"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

const (
	defaultConfigFile = "configs/config.yaml"
)

// Load 从指定文件路径加载配置
func Load(configFile string) (*Config, error) {
	return LoadWithEnv(configFile, "")
}

// LoadWithEnv 从指定文件路径加载配置并绑定环境变量
func LoadWithEnv(configFile string, envFile string) (*Config, error) {
	configFile = strings.TrimSpace(configFile)
	if configFile == "" {
		configFile = defaultConfigFile
	}

	// filepath.Abs() 将相对路径转换为绝对路径, 示例: "./configs/config.yaml"
	configFilename, err := filepath.Abs(configFile)
	if err != nil {
		return nil, ErrReadConfigFailed
	}

	if err := loadDotEnv(configFilename, envFile); err != nil {
		return nil, err
	}

	v := viper.New()
	v.SetConfigFile(configFilename)
	v.SetConfigType("yaml")

	// 将环境变量键名中的点替换为下划线, 示例: APP_NAME -> APP_NAME
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	// 自动从环境变量中加载配置
	v.AutomaticEnv()
	v.AllowEmptyEnv(true)

	if err := bindEnvironmentVariables(v); err != nil {
		return nil, err
	}

	if err := v.ReadInConfig(); err != nil {
		return nil, ErrReadConfigFailed
	}

	var cfg Config
	decodeHook := mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
	)
	decoderConfigOption := viper.DecodeHook(decodeHook)

	if err := v.UnmarshalExact(&cfg, decoderConfigOption); err != nil {
		return nil, ErrUnmarshalConfigFailed
	}
	if err := validateConfig(&cfg); err != nil {
		return nil, ErrValidateConfigFailed
	}
	return &cfg, nil
}
