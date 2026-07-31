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

// 服务角色常量(APP_ROLE 环境变量控制)
//
//	gateway: API 网关,处理业务 CRUD + 转发 AI/RAG 请求(默认,单体模式同此)
//	ai:      AI 服务,只承接 AI 工作流执行
//	rag:     RAG 服务,只承接向量检索和文档入库
const (
	RoleGateway = "gateway"
	RoleAI      = "ai"
	RoleRAG     = "rag"
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

	// 服务角色(由 APP_ROLE 环境变量控制,默认 gateway)
	// 微服务部署时,各容器通过 APP_ROLE 裁剪初始化范围,实现资源隔离
	role := cfg.App.Role
	if role == "" {
		role = RoleGateway
	}
	logger.Get().Infof("service role: %s (microservices mode=%v)", role, role != RoleGateway)

	// ============== 数据库初始化(所有角色共享) ==============
	// MySQL: 业务数据 + AI 工作流元数据 + RAG 文档元数据(所有角色都需要)
	mysqlDB, err := database.InitMySQL(cfg.MySQL)
	if err != nil {
		logger.Get().Fatalf("init mysql failed: %v", err)
	}

	// 自动迁移(仅 gateway 角色执行,避免多服务并发迁移冲突)
	if role == RoleGateway {
		if err := database.AutoMigrate(mysqlDB); err != nil {
			logger.Get().Fatalf("auto migrate failed: %v", err)
		}
	}

	// Redis: 缓存(所有角色共享)
	_, err = database.InitRedis(cfg.Redis)
	if err != nil {
		logger.Get().Warnf("init redis failed (some features may not work): %v", err)
	}

	// PostgreSQL + pgvector: 向量库(ai/rag 角色需要,gateway 不需要)
	if role == RoleAI || role == RoleRAG {
		_, err = database.InitPostgres(cfg.PG)
		if err != nil {
			logger.Get().Warnf("init postgres failed (RAG features may not work): %v", err)
		}
	}

	// 仅迁移模式
	if *migrateOnly {
		logger.Get().Info("migration completed, exiting")
		return
	}

	// 种子数据(仅 gateway 在开发模式下执行,避免多服务重复 seed)
	if role == RoleGateway && (*seedOnly || cfg.IsDev()) {
		if err := database.SeedData(mysqlDB); err != nil {
			logger.Get().Warnf("seed data failed: %v", err)
		}
		if *seedOnly {
			logger.Get().Info("seed completed, exiting")
			return
		}
	}

	// ============== 业务后台任务(仅 gateway 角色) ==============
	// 对账自动匹配 + 库存预警:业务定时任务,只在 gateway 启动避免重复执行
	if role == RoleGateway {
		reconSvc := finance.NewReconciliationService(mysqlDB)
		if m, d, err := reconSvc.AutoMatch(); err != nil {
			logger.Get().Warnf("reconciliation auto match failed: %v", err)
		} else {
			logger.Get().Infof("reconciliation auto match done: matched=%d, disputed=%d", m, d)
		}

		go func() {
			ticker := time.NewTicker(1 * time.Hour)
			defer ticker.Stop()
			alertSvc := inventory.NewAlertService(mysqlDB)
			_ = alertSvc.CheckAndTrigger()
			for range ticker.C {
				logger.Get().Info("scheduled inventory alert check triggered")
				if err := alertSvc.CheckAndTrigger(); err != nil {
					logger.Get().Warnf("scheduled alert check failed: %v", err)
				}
			}
		}()
	}

	// ============== AI 工作流调度器(gateway 单体模式 + ai 角色) ==============
	// rag 角色不需要 AI 工作流引擎,只处理向量检索
	if role == RoleGateway || role == RoleAI {
		aiEngine := ai.GetEngine(mysqlDB)
		if pgDB := database.GetPostgres(); pgDB != nil {
			aiEngine.SetPostgres(pgDB)
			aiEngine.SetEmbedder(ai.NewEmbeddingProvider(cfg.LLM))
		}
		if rdb := database.GetRedisSafe(); rdb != nil {
			aiEngine.SetRedis(rdb)
		}
		// 微服务模式:AI Service 通过 HTTP 调用 RAG Service(而非进程内调用)
		// 配置了 RAG_SERVICE_URL 时注入 RemoteRAGClient,实现服务解耦
		// 容错:RemoteRAGClient 内置熔断器 + 本地 TF-IDF 降级,确保 RAG Service 不可用时工作流仍可执行
		if role == RoleAI && cfg.Service.RAGServiceURL != "" {
			remoteClient := ai.NewRemoteRAGClient(cfg.Service.RAGServiceURL)
			// 注入本地 RAGService 作为降级检索器
			// 传 nil pgDB 强制 TF-IDF 模式,避免 AI Service 与 RAG Service 向量检索能力重叠
			// 仅当 RAG Service 不可用(熔断/网络故障)时,AI Service 才用 TF-IDF 兜底
			localRAG := ai.NewRAGService(mysqlDB, nil, cfg.LLM)
			if rdb := database.GetRedisSafe(); rdb != nil {
				localRAG.SetRedis(rdb)
			}
			remoteClient.SetFallback(localRAG)
			aiEngine.SetRAGSearcher(remoteClient)
			logger.Get().Infof("ai service: using remote rag client with TF-IDF fallback, target=%s",
				cfg.Service.RAGServiceURL)
		}
		scheduler := ai.NewScheduler(mysqlDB, aiEngine)
		go scheduler.Start()
		defer scheduler.Stop()
	}

	// ============== HTTP 服务(所有角色都启动,路由按角色裁剪) ==============
	// gateway: 注册业务路由 + 反向代理(配置了 ServiceURL 时)
	// ai:      只注册 AI 工作流相关路由
	// rag:     只注册 RAG 知识库 + 向量检索路由
	// NewRouterWithOptions 从 cfg.App.Role 读取角色并裁剪路由
	router := http.NewRouterWithOptions(mysqlDB, cfg)
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
