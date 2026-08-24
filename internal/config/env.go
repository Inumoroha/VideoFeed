package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

var environmentBindings = []struct {
	key string
	env string
}{
	{key: "database.url", env: "DATABASE_URL"},
	{key: "auth.jwt.secret", env: "JWT_SECRET"},
}

// loadDotEnv 加载显式或根据配置文件位置推导的 .env 文件。
func loadDotEnv(configFilename, envFilename string) error {
	if strings.TrimSpace(envFilename) != "" {
		path, err := filepath.Abs(envFilename)
		if err != nil {
			return fmt.Errorf("解析 .env 文件路径 %q 失败: %w", envFilename, err)
		}
		if err := godotenv.Load(path); err != nil {
			return fmt.Errorf("加载 .env 文件 %q 失败: %w", path, err)
		}
		return nil
	}

	path, err := resolveEnvFile(configFilename)
	if err != nil {
		return err
	}
	if path == "" {
		return nil
	}
	if err := godotenv.Load(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("加载 .env 文件 %q 失败: %w", path, err)
	}
	return nil
}

// resolveEnvFile 根据配置文件目录查找 .env，优先查找配置文件所在目录及其父目录。
// configFilename 配置文件路径，例如：/path/to/config.yaml
func resolveEnvFile(configFilename string) (string, error) {
	// filepath.Dir() 返回路径的目录部分，不包含文件名，示例：/path/to/file.yaml -> /path/to
	configDir := filepath.Dir(configFilename)
	candidates := []string{
		filepath.Join(configDir, ".env"),
		filepath.Join(filepath.Dir(configDir), ".env"),
	}
	for _, candidate := range candidates {
		// os.Stat() 返回文件或目录的文件信息，包括是否为目录、是否存在等
		info, err := os.Stat(candidate)
		if err == nil {
			if info.IsDir() {
				return "", fmt.Errorf(".env 路径是目录: %s", candidate)
			}
			return candidate, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("检查 .env 文件 %q 失败: %w", candidate, err)
		}
	}
	return "", nil
}

// bindEnvironmentVariables 将约定的环境变量绑定到 Viper 配置键。
func bindEnvironmentVariables(v *viper.Viper) error {
	for _, binding := range environmentBindings {
		if err := v.BindEnv(binding.key, binding.env); err != nil {
			return fmt.Errorf("绑定环境变量 %s 失败: %w", binding.env, err)
		}
	}
	return nil
}