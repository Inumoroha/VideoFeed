package config

import (
	"fmt"
	"strings"
)

// validateConfig 验证配置
func validateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if err := validateAppConfig(&cfg.App); err != nil {
		return err
	}
	if err := validateServerConfig(&cfg.Server); err != nil {
		return err
	}
	if err := validateDatabaseConfig(&cfg.Database); err != nil {
		return err
	}
	if err := validateAuthConfig(&cfg.Auth); err != nil {
		return err
	}
	if err := validateRedisConfig(&cfg.Redis); err != nil {
		return err
	}
	return nil
}

// validateAppConfig 验证应用配置
func validateAppConfig(cfg *AppConfig) error {
	if strings.TrimSpace(cfg.Name) == "" {
		return fmt.Errorf("app.name is empty")
	}
	if strings.TrimSpace(cfg.Env) == "" {
		return fmt.Errorf("app.env is empty")
	}
	if strings.TrimSpace(cfg.Version) == "" {
		return fmt.Errorf("app.version is empty")
	}
	return nil
}

// validateServerConfig 检查服务器主机和端口配置是否合法。
func validateServerConfig(cfg *ServerConfig) error {
	if strings.TrimSpace(cfg.Host) == "" {
		return fmt.Errorf("server.host is empty")
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	return nil
}

// validateDatabaseConfig 校验数据库驱动和连接地址，并拒绝未替换的环境变量占位符。
func validateDatabaseConfig(cfg *DatabaseConfig) error {
	if strings.TrimSpace(cfg.Driver) == "" {
		return fmt.Errorf("database.driver is empty")
	}
	url := strings.TrimSpace(cfg.URL)
	if url == "" {
		return fmt.Errorf("database.url is empty")
	}
	if strings.Contains(url, "${") {
		return fmt.Errorf("database.url contains unresolved environment variable")
	}
	return nil
}

// validateAuthConfig 校验认证配置
func validateAuthConfig(cfg *AuthConfig) error {
	if err := validateJWTConfig(&cfg.JWT); err != nil {
		return err
	}
	return nil
}

// validateRedisConfig 校验 Redis 地址和数据库编号是否合法。
func validateRedisConfig(cfg *RedisConfig) error {
	if strings.TrimSpace(cfg.Addr) == "" {
		return fmt.Errorf("redis.addr is empty")
	}
	if cfg.DB < 0 {
		return fmt.Errorf("redis.db must be non-negative")
	}
	return nil
}

// validateJWTConfig 校验JWT配置
func validateJWTConfig(cfg *JWTConfig) error {
	secret := strings.TrimSpace(cfg.Secret)
	if secret == "" {
		return fmt.Errorf("auth.jwt.secret is empty")
	}
	if strings.Contains(secret, "${") {
		return fmt.Errorf("auth.jwt.secret contains unresolved environment variable")
	}
	if secret == "replace-with-strong-secret" || len([]byte(secret)) < 32 {
		return fmt.Errorf("auth.jwt.secret must contain at least 32 bytes")
	}

	if cfg.AccessTTL <= 0 {
		return fmt.Errorf("auth.jwt.access_ttl must be a positive duration")
	}
	if cfg.RefreshTTL <= 0 {
		return fmt.Errorf("auth.jwt.refresh_ttl must be a positive duration")
	}
	if cfg.RefreshTTL <= cfg.AccessTTL {
		return fmt.Errorf("auth.jwt.refresh_ttl must be greater than auth.jwt.access_ttl")
	}
	return nil
}
