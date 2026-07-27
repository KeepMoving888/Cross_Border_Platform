package database

import (
	"context"
	"fmt"
	"time"

	"github.com/cb-platform/internal/pkg/config"
	"github.com/cb-platform/internal/pkg/logger"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var (
	mysqlDB *gorm.DB
	pgDB    *gorm.DB
	redisClient *redis.Client
)

// InitMySQL 初始化 MySQL 连接(业务数据)
func InitMySQL(cfg config.MySQLConfig) (*gorm.DB, error) {
	logLevel := gormlogger.Warn
	if config.Get().IsDev() {
		logLevel = gormlogger.Info
	}

	db, err := gorm.Open(mysql.Open(cfg.DSN()), &gorm.Config{
		Logger:      gormlogger.Default.LogMode(logLevel),
		PrepareStmt: true,
	})
	if err != nil {
		return nil, fmt.Errorf("connect mysql failed: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(10 * time.Minute)

	// 验证连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping mysql failed: %w", err)
	}

	mysqlDB = db
	logger.Get().Infow("mysql connected", "host", cfg.Host, "port", cfg.Port, "db", cfg.DB)
	return db, nil
}

// InitPostgres 初始化 PostgreSQL(向量库)
func InitPostgres(cfg config.PGConfig) (*gorm.DB, error) {
	logLevel := gormlogger.Warn
	if config.Get().IsDev() {
		logLevel = gormlogger.Info
	}

	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
		Logger:      gormlogger.Default.LogMode(logLevel),
		PrepareStmt: true,
	})
	if err != nil {
		return nil, fmt.Errorf("connect postgres failed: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetMaxOpenConns(30)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// 启用 pgvector 扩展
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = db.WithContext(ctx).Exec("CREATE EXTENSION IF NOT EXISTS vector").Error

	pgDB = db
	logger.Get().Infow("postgres connected", "host", cfg.Host, "port", cfg.Port, "db", cfg.DB)
	return db, nil
}

// InitRedis 初始化 Redis
func InitRedis(cfg config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr(),
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     20,
		MinIdleConns: 5,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis failed: %w", err)
	}

	redisClient = client
	logger.Get().Infow("redis connected", "addr", cfg.Addr(), "db", cfg.DB)
	return client, nil
}

// GetMySQL 获取 MySQL 实例
func GetMySQL() *gorm.DB {
	if mysqlDB == nil {
		logger.Get().Fatalf("mysql not initialized")
	}
	return mysqlDB
}

// GetPostgres 获取 PostgreSQL 实例
func GetPostgres() *gorm.DB {
	if pgDB == nil {
		// 向量库可选,未初始化时返回 nil
		return nil
	}
	return pgDB
}

// GetRedis 获取 Redis 实例
func GetRedis() *redis.Client {
	if redisClient == nil {
		logger.Get().Fatalf("redis not initialized")
	}
	return redisClient
}

// Close 关闭所有数据库连接
func Close() {
	if mysqlDB != nil {
		if sqlDB, err := mysqlDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	if pgDB != nil {
		if sqlDB, err := pgDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	if redisClient != nil {
		_ = redisClient.Close()
	}
	logger.Get().Info("database connections closed")
}

// HealthCheck 数据库健康检查
func HealthCheck() map[string]string {
	status := make(map[string]string)

	if mysqlDB != nil {
		if sqlDB, err := mysqlDB.DB(); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := sqlDB.PingContext(ctx); err == nil {
				status["mysql"] = "ok"
			} else {
				status["mysql"] = fmt.Sprintf("error: %v", err)
			}
		}
	} else {
		status["mysql"] = "not initialized"
	}

	if pgDB != nil {
		if sqlDB, err := pgDB.DB(); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := sqlDB.PingContext(ctx); err == nil {
				status["postgres"] = "ok"
			} else {
				status["postgres"] = fmt.Sprintf("error: %v", err)
			}
		}
	} else {
		status["postgres"] = "not initialized"
	}

	if redisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := redisClient.Ping(ctx).Err(); err == nil {
			status["redis"] = "ok"
		} else {
			status["redis"] = fmt.Sprintf("error: %v", err)
		}
	} else {
		status["redis"] = "not initialized"
	}

	return status
}

// 全局 zap 字段,避免 unused 警告
var _ = zap.Field{}
