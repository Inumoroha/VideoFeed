package config

import "time"

// Config 保存应用运行所需的全部配置项。
type Config struct {
	Internal InternalConfig `mapstructure:"internal"`
	App      AppConfig      `mapstructure:"app"`
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Auth     AuthConfig     `mapstructure:"auth"`
	Redis    RedisConfig    `mapstructure:"redis"`
	RabbitMQ RabbitMQConfig `mapstructure:"rabbitmq"`
}

// AppConfig 是应用配置结构体
type AppConfig struct {
	Name    string `mapstructure:"name"`
	Env     string `mapstructure:"env"`
	Version string `mapstructure:"version"`
}

// ServerConfig 是服务器配置结构体
type ServerConfig struct {
	Host              string        `mapstructure:"host"`
	Port              int           `mapstructure:"port"`
	ReadHeaderTimeout time.Duration `mapstructure:"read_header_timeout"`
	ReadTimeout       time.Duration `mapstructure:"read_timeout"`
	WriteTimeout      time.Duration `mapstructure:"write_timeout"`
	IdleTimeout       time.Duration `mapstructure:"idle_timeout"`
	ShutdownTimeout   time.Duration `mapstructure:"shutdown_timeout"`
}

// DatabaseConfig 是数据库配置结构体
type DatabaseConfig struct {
	Driver string `mapstructure:"driver"`
	URL    string `mapstructure:"url"`
}

// AuthConfig 是认证配置结构体
type AuthConfig struct {
	JWT JWTConfig `mapstructure:"jwt"`
}

// JWTConfig 是JWT配置结构体
type JWTConfig struct {
	Secret     string        `mapstructure:"secret"`
	AccessTTL  time.Duration `mapstructure:"access_ttl"`
	RefreshTTL time.Duration `mapstructure:"refresh_ttl"`
}

// RedisConfig 保存 Redis 服务的连接地址、认证密码和数据库编号。
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// RabbitMQConfig 保存互动、视频与曝光事件队列的连接和拓扑配置。
type RabbitMQConfig struct {
	VideoEmbeddingQueue      string `mapstructure:"video_embedding_queue"`
	ExposureExchange         string `mapstructure:"exposure_exchange"`
	ViewEventRecordedQueue   string `mapstructure:"view_event_recorded_queue"`
	ViewEventRecordedRouting string `mapstructure:"view_event_recorded_routing"`
	VideoExchange            string `mapstructure:"video_exchange"`
	VideoPublishedQueue      string `mapstructure:"video_published_queue"`
	VideoPublishedRouting    string `mapstructure:"video_published_routing"`
	URL                      string `mapstructure:"url"`
	InteractionExchange      string `mapstructure:"interaction_exchange"`
	ActionChangedQueue       string `mapstructure:"action_changed_queue"`
	ActionChangedRouting     string `mapstructure:"action_changed_routing"`
}

// InternalConfig 保存内部消息接口的独立鉴权令牌，空值禁止调用。
type InternalConfig struct {
	Token string `mapstructure:"token"`
}
