package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config 全局配置
type Config struct {
	App      AppConfig      `mapstructure:"app"`
	MySQL    MySQLConfig    `mapstructure:"mysql"`
	PG       PGConfig       `mapstructure:"pg"`
	Redis    RedisConfig    `mapstructure:"redis"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	LLM      LLMConfig      `mapstructure:"llm"`
	Asynq    AsynqConfig    `mapstructure:"asynq"`
	Log      LogConfig      `mapstructure:"log"`
}

type AppConfig struct {
	Env  string `mapstructure:"env"`
	Port int    `mapstructure:"port"`
	Name string `mapstructure:"name"`
}

type MySQLConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DB       string `mapstructure:"db"`
}

func (c MySQLConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local&time_zone=%%27%%2B00%%3A00%%27",
		c.User, c.Password, c.Host, c.Port, c.DB)
}

type PGConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DB       string `mapstructure:"db"`
	SSLMode  string `mapstructure:"sslmode"`
}

func (c PGConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DB, c.SSLMode)
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

func (c RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

type JWTConfig struct {
	Secret       string `mapstructure:"secret"`
	ExpireHours  int    `mapstructure:"expire_hours"`
}

type LLMConfig struct {
	Provider string `mapstructure:"provider"`
	APIKey   string `mapstructure:"api_key"`
	BaseURL  string `mapstructure:"base_url"`
	Model    string `mapstructure:"model"`
}

type AsynqConfig struct {
	Concurrency int `mapstructure:"concurrency"`
}

type LogConfig struct {
	Level string `mapstructure:"level"`
	Dir   string `mapstructure:"dir"`
}

var cfg *Config

// Load 加载配置(.env 文件 + 环境变量)
func Load() (*Config, error) {
	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// 读取 .env 文件(如存在)
	v.SetConfigName(".env")
	v.SetConfigType("env")
	v.AddConfigPath(".")
	v.AddConfigPath("./..")
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config file failed: %w", err)
		}
	}

	// 设置默认值
	v.SetDefault("app.env", "development")
	v.SetDefault("app.port", 8080)
	v.SetDefault("app.name", "cb-platform")
	v.SetDefault("mysql.host", "127.0.0.1")
	v.SetDefault("mysql.port", 3306)
	v.SetDefault("pg.sslmode", "disable")
	v.SetDefault("redis.port", 6379)
	v.SetDefault("redis.db", 0)
	v.SetDefault("jwt.expire_hours", 24)
	v.SetDefault("asynq.concurrency", 10)
	v.SetDefault("log.level", "info")
	v.SetDefault("log.dir", "logs")

	c := &Config{}
	if err := v.Unmarshal(c); err != nil {
		return nil, fmt.Errorf("unmarshal config failed: %w", err)
	}

	// 兼容 .env 文件的扁平 key(如 MYSQL_HOST)
	c.MySQL.Host = getOr(v, "MYSQL_HOST", c.MySQL.Host)
	c.MySQL.Port = getOrInt(v, "MYSQL_PORT", c.MySQL.Port)
	c.MySQL.User = getOr(v, "MYSQL_USER", c.MySQL.User)
	c.MySQL.Password = getOr(v, "MYSQL_PASSWORD", c.MySQL.Password)
	c.MySQL.DB = getOr(v, "MYSQL_DB", c.MySQL.DB)

	c.PG.Host = getOr(v, "PG_HOST", c.PG.Host)
	c.PG.Port = getOrInt(v, "PG_PORT", c.PG.Port)
	c.PG.User = getOr(v, "PG_USER", c.PG.User)
	c.PG.Password = getOr(v, "PG_PASSWORD", c.PG.Password)
	c.PG.DB = getOr(v, "PG_DB", c.PG.DB)
	c.PG.SSLMode = getOr(v, "PG_SSLMODE", c.PG.SSLMode)

	c.Redis.Host = getOr(v, "REDIS_HOST", c.Redis.Host)
	c.Redis.Port = getOrInt(v, "REDIS_PORT", c.Redis.Port)
	c.Redis.Password = getOr(v, "REDIS_PASSWORD", c.Redis.Password)
	c.Redis.DB = getOrInt(v, "REDIS_DB", c.Redis.DB)

	c.JWT.Secret = getOr(v, "JWT_SECRET", c.JWT.Secret)
	c.JWT.ExpireHours = getOrInt(v, "JWT_EXPIRE_HOURS", c.JWT.ExpireHours)

	c.LLM.Provider = getOr(v, "LLM_PROVIDER", c.LLM.Provider)
	c.LLM.APIKey = getOr(v, "LLM_API_KEY", c.LLM.APIKey)
	c.LLM.BaseURL = getOr(v, "LLM_BASE_URL", c.LLM.BaseURL)
	c.LLM.Model = getOr(v, "LLM_MODEL", c.LLM.Model)

	c.App.Env = getOr(v, "APP_ENV", c.App.Env)
	c.App.Port = getOrInt(v, "APP_PORT", c.App.Port)
	c.App.Name = getOr(v, "APP_NAME", c.App.Name)

	c.Log.Level = getOr(v, "LOG_LEVEL", c.Log.Level)
	c.Log.Dir = getOr(v, "LOG_DIR", c.Log.Dir)

	cfg = c
	return c, nil
}

func getOr(v *viper.Viper, key, def string) string {
	if val := v.GetString(key); val != "" {
		return val
	}
	return def
}

func getOrInt(v *viper.Viper, key string, def int) int {
	if val := v.GetInt(key); val != 0 {
		return val
	}
	return def
}

// Get 获取已加载的全局配置
func Get() *Config {
	if cfg == nil {
		c, err := Load()
		if err != nil {
			panic(fmt.Sprintf("config not loaded: %v", err))
		}
		return c
	}
	return cfg
}

// IsDev 是否开发环境
func (c *Config) IsDev() bool {
	return c.App.Env == "development" || c.App.Env == "dev"
}

// IsProd 是否生产环境
func (c *Config) IsProd() bool {
	return c.App.Env == "production" || c.App.Env == "prod"
}
