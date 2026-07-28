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
	mysqlDB     *gorm.DB
	pgDB        *gorm.DB
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

	// 为 knowledge_chunks 添加 BM25 全文检索列(tsvector)+ GIN 索引
	// 与向量检索互补:向量擅长语义近似,tsvector 擅长精确关键词(型号/SKU)匹配
	// 使用 simple 配置避免英文词干提取干扰中文匹配
	setupBM25Search(ctx, db)

	pgDB = db
	logger.Get().Infow("postgres connected", "host", cfg.Host, "port", cfg.Port, "db", cfg.DB)
	return db, nil
}

// setupBM25Search 在 PostgreSQL 上创建 BM25 全文检索所需的列和索引
// 幂等操作:列/索引已存在时静默跳过
func setupBM25Search(ctx context.Context, db *gorm.DB) {
	// 1. 添加 content_tsv 列(tsvector 类型)
	_ = db.WithContext(ctx).Exec(
		"ALTER TABLE knowledge_chunks ADD COLUMN IF NOT EXISTS content_tsv tsvector",
	).Error

	// 2. 填充已有数据的 content_tsv(对存量分块建全文索引)
	_ = db.WithContext(ctx).Exec(
		"UPDATE knowledge_chunks SET content_tsv = to_tsvector('simple', coalesce(content, '')) WHERE content_tsv IS NULL",
	).Error

	// 3. 创建 GIN 索引(加速 @@ tsquery 查询)
	_ = db.WithContext(ctx).Exec(
		"CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_content_tsv ON knowledge_chunks USING GIN (content_tsv)",
	).Error

	// 4. 创建触发器:插入/更新 content 时自动维护 content_tsv
	// 避免应用层手动维护,保证全文索引与内容一致
	_ = db.WithContext(ctx).Exec(`
		CREATE OR REPLACE FUNCTION knowledge_chunks_tsv_trigger() RETURNS trigger AS $$
		BEGIN
			NEW.content_tsv := to_tsvector('simple', coalesce(NEW.content, ''));
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql
	`).Error
	_ = db.WithContext(ctx).Exec(`
		DROP TRIGGER IF EXISTS trg_knowledge_chunks_tsv ON knowledge_chunks
	`).Error
	_ = db.WithContext(ctx).Exec(`
		CREATE TRIGGER trg_knowledge_chunks_tsv
		BEFORE INSERT OR UPDATE ON knowledge_chunks
		FOR EACH ROW EXECUTE FUNCTION knowledge_chunks_tsv_trigger()
	`).Error
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

// GetRedisSafe 安全获取 Redis 实例(未初始化时返回 nil,不触发 Fatal)
// 用于可选依赖场景(如 RAG 缓存,Redis 不可用时自动降级为无缓存)
func GetRedisSafe() *redis.Client {
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
