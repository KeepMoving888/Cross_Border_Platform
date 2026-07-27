package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMySQLConfig_DSN(t *testing.T) {
	cfg := MySQLConfig{
		Host:     "127.0.0.1",
		Port:     3306,
		User:     "root",
		Password: "secret",
		DB:       "cb_platform",
	}
	dsn := cfg.DSN()
	if dsn == "" {
		t.Error("expected non-empty DSN")
	}
	if !contains(dsn, "root") || !contains(dsn, "secret") || !contains(dsn, "127.0.0.1") || !contains(dsn, "3306") || !contains(dsn, "cb_platform") {
		t.Errorf("DSN missing required fields: %s", dsn)
	}
}

func TestPGConfig_DSN(t *testing.T) {
	cfg := PGConfig{
		Host:     "127.0.0.1",
		Port:     5432,
		User:     "postgres",
		Password: "secret",
		DB:       "cb_vector",
		SSLMode:  "disable",
	}
	dsn := cfg.DSN()
	if dsn == "" {
		t.Error("expected non-empty DSN")
	}
	if !contains(dsn, "host=127.0.0.1") || !contains(dsn, "port=5432") {
		t.Errorf("DSN missing host/port: %s", dsn)
	}
	if !contains(dsn, "sslmode=disable") {
		t.Errorf("DSN missing sslmode: %s", dsn)
	}
}

func TestRedisConfig_Addr(t *testing.T) {
	cfg := RedisConfig{Host: "127.0.0.1", Port: 6379}
	if got := cfg.Addr(); got != "127.0.0.1:6379" {
		t.Errorf("expected '127.0.0.1:6379', got %s", got)
	}
}

func TestConfig_IsDev(t *testing.T) {
	tests := []struct {
		env    string
		expect bool
	}{
		{"development", true},
		{"dev", true},
		{"production", false},
		{"test", false},
		{"", false},
	}
	for _, tt := range tests {
		c := &Config{App: AppConfig{Env: tt.env}}
		if got := c.IsDev(); got != tt.expect {
			t.Errorf("env=%q: expected %v, got %v", tt.env, tt.expect, got)
		}
	}
}

func TestConfig_IsProd(t *testing.T) {
	tests := []struct {
		env    string
		expect bool
	}{
		{"production", true},
		{"prod", true},
		{"development", false},
		{"test", false},
		{"", false},
	}
	for _, tt := range tests {
		c := &Config{App: AppConfig{Env: tt.env}}
		if got := c.IsProd(); got != tt.expect {
			t.Errorf("env=%q: expected %v, got %v", tt.env, tt.expect, got)
		}
	}
}

func TestLoad_Defaults(t *testing.T) {
	// 创建临时工作目录,避免读到项目根目录的 .env
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	// 重置全局配置
	cfg = nil

	// 清理可能影响的环境变量
	envKeys := []string{
		"APP_ENV", "APP_PORT", "APP_NAME",
		"MYSQL_HOST", "MYSQL_PORT", "MYSQL_USER", "MYSQL_PASSWORD", "MYSQL_DB",
		"REDIS_HOST", "REDIS_PORT", "REDIS_PASSWORD", "REDIS_DB",
		"PG_HOST", "PG_PORT", "PG_USER", "PG_PASSWORD", "PG_DB", "PG_SSLMODE",
		"JWT_SECRET", "JWT_EXPIRE_HOURS",
		"LLM_PROVIDER", "LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL",
		"LOG_LEVEL", "LOG_DIR",
	}
	for _, k := range envKeys {
		old := os.Getenv(k)
		os.Unsetenv(k)
		defer os.Setenv(k, old)
	}

	c, err := Load()
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}

	if c.App.Env != "development" {
		t.Errorf("expected default env=development, got %s", c.App.Env)
	}
	if c.App.Port != 8080 {
		t.Errorf("expected default port=8080, got %d", c.App.Port)
	}
	if c.App.Name != "cb-platform" {
		t.Errorf("expected default name=cb-platform, got %s", c.App.Name)
	}
	if c.MySQL.Host != "127.0.0.1" {
		t.Errorf("expected default mysql.host=127.0.0.1, got %s", c.MySQL.Host)
	}
	if c.MySQL.Port != 3306 {
		t.Errorf("expected default mysql.port=3306, got %d", c.MySQL.Port)
	}
	if c.PG.SSLMode != "disable" {
		t.Errorf("expected default pg.sslmode=disable, got %s", c.PG.SSLMode)
	}
	if c.Redis.Port != 6379 {
		t.Errorf("expected default redis.port=6379, got %d", c.Redis.Port)
	}
	if c.JWT.ExpireHours != 24 {
		t.Errorf("expected default jwt.expire_hours=24, got %d", c.JWT.ExpireHours)
	}
	if c.Log.Level != "info" {
		t.Errorf("expected default log.level=info, got %s", c.Log.Level)
	}
}

func TestLoad_FromEnvFile(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	cfg = nil

	// 清理相关环境变量
	for _, k := range []string{"APP_PORT", "MYSQL_HOST", "LLM_PROVIDER"} {
		os.Unsetenv(k)
	}

	// 写入 .env 文件
	envContent := `APP_PORT=9090
MYSQL_HOST=mysql.example.com
LLM_PROVIDER=glm
LLM_API_KEY=test-key-123
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatalf("write .env failed: %v", err)
	}

	c, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if c.App.Port != 9090 {
		t.Errorf("expected port 9090 from .env, got %d", c.App.Port)
	}
	if c.MySQL.Host != "mysql.example.com" {
		t.Errorf("expected mysql host from .env, got %s", c.MySQL.Host)
	}
	if c.LLM.Provider != "glm" {
		t.Errorf("expected llm provider glm from .env, got %s", c.LLM.Provider)
	}
	if c.LLM.APIKey != "test-key-123" {
		t.Errorf("expected llm api key from .env, got %s", c.LLM.APIKey)
	}
}

func TestGet_AfterLoad(t *testing.T) {
	// Load 后 Get 应返回同一实例
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmpDir)
	cfg = nil

	c, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	got := Get()
	if got != c {
		t.Error("Get() should return same instance as Load()")
	}
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
