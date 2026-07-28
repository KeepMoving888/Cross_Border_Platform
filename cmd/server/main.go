package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cb-platform/internal/application/ai"
	"github.com/cb-platform/internal/application/finance"
	"github.com/cb-platform/internal/application/inventory"
	"github.com/cb-platform/internal/interfaces/http"
	"github.com/cb-platform/internal/pkg/config"
	"github.com/cb-platform/internal/pkg/database"
	"github.com/cb-platform/internal/pkg/logger"
)

// 版本信息(由 -ldflags 在构建时注入)
var (
	Version     = "dev"
	BuildCommit = "unknown"
	BuildDate   = "unknown"
)

// BuildInfo 构建信息
type BuildInfo struct {
	Version     string
	BuildCommit string
	BuildDate   string
}

func main() {
	// 命令行参数
	migrateOnly := flag.Bool("migrate", false, "only run database migration and exit")
	seedOnly := flag.Bool("seed", false, "run database migration and seed data, then exit")
	showVersion := flag.Bool("version", false, "show version info and exit")
	flag.Parse()

	// 版本信息
	if *showVersion {
		fmt.Printf("cb-platform %s (commit: %s, built: %s)\n", Version, BuildCommit, BuildDate)
		return
	}

	info := BuildInfo{
		Version:     Version,
		BuildCommit: BuildCommit,
		BuildDate:   BuildDate,
	}

	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("load config failed: %v\n", err)
		os.Exit(1)
	}

	// 初始化日志
	if err := logger.Init(cfg.Log.Level, cfg.Log.Dir); err != nil {
		fmt.Printf("init logger failed: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Get().Infof("=== %s starting (version: %s, env: %s) ===",
		cfg.App.Name, info.Version, cfg.App.Env)
	if info.BuildCommit != "unknown" {
		logger.Get().Infof("build: commit=%s date=%s", info.BuildCommit, info.BuildDate)
	}

	// 初始化数据库
	mysqlDB, err := database.InitMySQL(cfg.MySQL)
	if err != nil {
		logger.Get().Fatalf("init mysql failed: %v", err)
	}

	// 自动迁移
	if err := database.AutoMigrate(mysqlDB); err != nil {
		logger.Get().Fatalf("auto migrate failed: %v", err)
	}

	// 初始化 Redis
	_, err = database.InitRedis(cfg.Redis)
	if err != nil {
		logger.Get().Warnf("init redis failed (some features may not work): %v", err)
	}

	// 初始化 PG(向量库,可选)
	_, err = database.InitPostgres(cfg.PG)
	if err != nil {
		logger.Get().Warnf("init postgres failed (RAG features may not work): %v", err)
	}

	// 仅迁移模式
	if *migrateOnly {
		logger.Get().Info("migration completed, exiting")
		return
	}

	// 初始化种子数据
	if *seedOnly || cfg.IsDev() {
		if err := database.SeedData(mysqlDB); err != nil {
			logger.Get().Warnf("seed data failed: %v", err)
		}
		if *seedOnly {
			logger.Get().Info("seed completed, exiting")
			return
		}
	}

	// 对账自动匹配:启动时执行一次(seed 之后),自动匹配账单与采购单并标记差异
	reconSvc := finance.NewReconciliationService(mysqlDB)
	if m, d, err := reconSvc.AutoMatch(); err != nil {
		logger.Get().Warnf("reconciliation auto match failed: %v", err)
	} else {
		logger.Get().Infof("reconciliation auto match done: matched=%d, disputed=%d", m, d)
	}

	// 库存预警定时检查：启动时先执行一次，之后每小时执行一次
	// 检查低库存/无货记录，自动创建采购询价 + 消息
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		// 启动时先执行一次
		alertSvc := inventory.NewAlertService(mysqlDB)
		_ = alertSvc.CheckAndTrigger()
		for range ticker.C {
			logger.Get().Info("scheduled inventory alert check triggered")
			if err := alertSvc.CheckAndTrigger(); err != nil {
				logger.Get().Warnf("scheduled alert check failed: %v", err)
			}
		}
	}()

	// 启动 AI 工作流调度器
	aiEngine := ai.GetEngine(mysqlDB)
	// 注入 PostgreSQL 和 Embedder(启用 RAG 向量检索)
	if pgDB := database.GetPostgres(); pgDB != nil {
		aiEngine.SetPostgres(pgDB)
		aiEngine.SetEmbedder(ai.NewEmbeddingProvider(cfg.LLM))
	}
	// 注入 Redis(启用 RAG 检索结果缓存,可选)
	if rdb := database.GetRedisSafe(); rdb != nil {
		aiEngine.SetRedis(rdb)
	}
	scheduler := ai.NewScheduler(mysqlDB, aiEngine)
	go scheduler.Start()
	defer scheduler.Stop()

	// 启动 HTTP 服务
	// 微服务模式:根据 cfg.Service 启用反向代理转发(AI/RAG 请求转发到下游服务)
	// 单体模式:cfg.Service 为空,所有路由本地处理
	router := http.NewRouter(mysqlDB, cfg)
	addr := fmt.Sprintf(":%d", cfg.App.Port)

	srv := &http.ServerImpl{
		Router: router,
		Addr:   addr,
	}

	// 优雅启停
	go func() {
		logger.Get().Infof("http server listening on %s", addr)
		if err := srv.Start(); err != nil {
			logger.Get().Fatalf("server start failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Get().Info("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Stop(ctx); err != nil {
		logger.Get().Errorf("server forced to shutdown: %v", err)
	}

	database.Close()
	logger.Get().Info("=== server exited ===")
}
