package config

import "testing"

// TestValidateRedisConfig 验证 Redis 配置的地址和数据库编号校验规则。
func TestValidateRedisConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  RedisConfig
		wantErr string
	}{
		{
			name:    "empty address",
			config:  RedisConfig{DB: 0},
			wantErr: "redis.addr is empty",
		},
		{
			name:    "negative database",
			config:  RedisConfig{Addr: "localhost:6379", DB: -1},
			wantErr: "redis.db must be non-negative",
		},
		{
			name:   "valid config",
			config: RedisConfig{Addr: "localhost:6379", Password: "secret", DB: 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRedisConfig(&tt.config)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateRedisConfig() error = %v", err)
				}
				return
			}
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("validateRedisConfig() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}
