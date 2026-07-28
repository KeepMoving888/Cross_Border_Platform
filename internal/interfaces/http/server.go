package http

import (
	"context"
	"net/http"

	"github.com/cb-platform/internal/interfaces/http/handler"
	"github.com/cb-platform/internal/pkg/config"
	"github.com/cb-platform/internal/pkg/database"
	"github.com/cb-platform/internal/pkg/logger"
	"github.com/cb-platform/internal/pkg/middleware"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// 服务角色常量(与 cmd/server/main.go 保持一致)
const (
	RoleGateway = "gateway"
	RoleAI      = "ai"
	RoleRAG     = "rag"
)

// promhttpHandler 返回 Prometheus HTTP Handler
func promhttpHandler() http.Handler {
	return promhttp.Handler()
}

// ServerImpl HTTP 服务封装
type ServerImpl struct {
	Router *gin.Engine
	Addr   string
	server *http.Server
}

// Start 启动 HTTP 服务
func (s *ServerImpl) Start() error {
	s.server = &http.Server{
		Addr:    s.Addr,
		Handler: s.Router,
	}
	return s.server.ListenAndServe()
}

// Stop 优雅停止
func (s *ServerImpl) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

// NewRouter 创建并配置路由(兼容旧调用,等同于 NewRouterWithOptions)
// cfg: 全局配置,微服务模式下根据 ServiceConfig 启用反向代理转发
func NewRouter(db *gorm.DB, cfg ...*config.Config) *gin.Engine {
	return NewRouterWithOptions(db, cfg...)
}

// NewRouterWithOptions 创建并配置路由,按 cfg.App.Role 裁剪路由和初始化
//
//	role=gateway: 全部业务路由 + AI/RAG 路由(本地处理) + 反向代理(配置了 ServiceURL 时)
//	role=ai:      仅 auth + AI 工作流路由(workflows/runs/prompts/analyze/generate/reply)
//	role=rag:     仅 auth + RAG 路由(knowledge-bases/rag/search)
func NewRouterWithOptions(db *gorm.DB, cfg ...*config.Config) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	// 解析角色(从 cfg.App.Role 读取,默认 gateway)
	role := RoleGateway
	var svcCfg config.ServiceConfig
	if len(cfg) > 0 && cfg[0] != nil {
		if cfg[0].App.Role != "" {
			role = cfg[0].App.Role
		}
		svcCfg = cfg[0].Service
	}

	r := gin.New()

	// 全局中间件
	r.Use(middleware.TraceID())
	r.Use(middleware.Recovery())
	r.Use(middleware.Logger())
	r.Use(middleware.Metrics())
	r.Use(middleware.CORS())

	// 健康检查(无需鉴权,所有角色都暴露)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"service": "cb-platform",
			"role":    role,
			"db":      database.HealthCheck(),
		})
	})
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	})

	// Prometheus 指标端点(所有角色都暴露,通过 job 标签区分)
	r.GET("/metrics", gin.WrapH(promhttpHandler()))

	// 微服务模式:仅 gateway 角色启用反向代理(ai/rag 是下游服务,不需要转发)
	if role == RoleGateway {
		proxyMW := NewProxyMiddleware(svcCfg)
		if proxyMW.Enabled() {
			proxyMW.Register(r)
			logger.Get().Info("microservices mode: proxy enabled")
		}
	}

	// 注册业务路由(按角色裁剪)
	registerRoutes(r, db, role)

	// 404
	r.NoRoute(func(c *gin.Context) {
		c.JSON(404, gin.H{"code": 404, "message": "资源不存在"})
	})

	logger.Get().Info("routes registered",
		zap.String("role", role),
		zap.Int("routes", len(r.Routes())))

	return r
}

// registerRoutes 注册业务路由,按角色裁剪
//
//	gateway: 全部业务路由(auth/users/messages/suppliers/products/purchases/inventory/finance/ai/dashboard/platform)
//	ai:      仅 auth + AI 工作流相关(workflows/runs/prompts/analyze/generate/reply)
//	rag:     仅 auth + RAG 知识库(knowledge-bases/rag)
func registerRoutes(r *gin.Engine, db *gorm.DB, role string) {
	v1 := r.Group("/api/v1")

	// 认证路由(所有角色都需要,下游服务接收 Gateway 转发的请求后仍需校验 JWT)
	{
		auth := v1.Group("/auth")
		h := handler.NewAuthHandler(db)
		auth.POST("/login", h.Login)
		auth.POST("/register", h.Register)
		auth.POST("/refresh", h.RefreshToken)
	}

	// gateway 角色:注册全部业务路由
	if role == RoleGateway {
		registerGatewayRoutes(v1, db)
		return
	}

	// ai/rag 角色:只注册 auth + 各自负责的 ai 子路由
	authRequired := v1.Group("")
	authRequired.Use(middleware.Auth())

	switch role {
	case RoleAI:
		registerAIRoutes(authRequired, db)
	case RoleRAG:
		registerRAGRoutes(authRequired, db)
	}
}

// registerGatewayRoutes 注册 gateway 角色的全部业务路由(除 auth 外)
func registerGatewayRoutes(v1 *gin.RouterGroup, db *gorm.DB) {
	authRequired := v1.Group("")
	authRequired.Use(middleware.Auth())
	{
		// 用户
		user := authRequired.Group("/users")
		{
			h := handler.NewUserHandler(db)
			user.GET("/me", h.GetCurrentUser)
			user.PUT("/me", h.UpdateCurrentUser)
			user.PUT("/me/password", h.ChangePassword)
			user.GET("", middleware.RequireRole("admin", "manager"), h.ListUsers)
			user.GET("/:id", middleware.RequireRole("admin", "manager"), h.GetUser)
			user.PUT("/:id", middleware.RequireRole("admin"), h.UpdateUser)
			user.DELETE("/:id", middleware.RequireRole("admin"), h.DeleteUser)
		}

		// 消息中心
		messages := authRequired.Group("/messages")
		{
			h := handler.NewMessageHandler(db)
			messages.GET("", h.List)
			messages.GET("/unread-count", h.UnreadCount)
			messages.PUT("/:id/read", h.MarkRead)
			messages.POST("/read-all", h.MarkAllRead)
		}

		// 供应商
		suppliers := authRequired.Group("/suppliers")
		{
			h := handler.NewSupplierHandler(db)
			suppliers.GET("", h.List)
			suppliers.GET("/:id", h.Get)
			suppliers.POST("", middleware.RequireRole("admin", "manager"), h.Create)
			suppliers.PUT("/:id", middleware.RequireRole("admin", "manager"), h.Update)
			suppliers.DELETE("/:id", middleware.RequireRole("admin"), h.Delete)
			suppliers.GET("/:id/products", h.ListProducts)
			suppliers.POST("/:id/products", middleware.RequireRole("admin", "manager"), h.AddProduct)
		}

		// 选品管理
		products := authRequired.Group("/products")
		{
			h := handler.NewProductHandler(db)
			products.GET("", h.List)
			products.GET("/:id", h.Get)
			products.POST("", middleware.RequireRole("admin", "manager"), h.Create)
			products.PUT("/:id", middleware.RequireRole("admin", "manager"), h.Update)
			products.DELETE("/:id", middleware.RequireRole("admin"), h.Delete)
			products.POST("/:id/analyze", h.Analyze)
			products.PUT("/:id/stage", middleware.RequireRole("admin", "manager"), h.ChangeStage)
			products.GET("/:id/trends", h.ListTrends)
			products.POST("/:id/competitors", middleware.RequireRole("admin", "manager"), h.AddCompetitor)
			products.GET("/:id/competitors", h.ListCompetitors)
		}

		// 采购管理
		purchases := authRequired.Group("/purchases")
		{
			h := handler.NewPurchaseHandler(db)
			purchases.GET("/inquiries", h.ListInquiries)
			purchases.GET("/inquiries/:id", h.GetInquiry)
			purchases.POST("/inquiries", middleware.RequireRole("admin", "manager"), h.CreateInquiry)
			purchases.PUT("/inquiries/:id", middleware.RequireRole("admin", "manager"), h.UpdateInquiry)
			purchases.DELETE("/inquiries/:id", middleware.RequireRole("admin"), h.DeleteInquiry)
			purchases.POST("/inquiries/:id/close", middleware.RequireRole("admin", "manager"), h.CloseInquiry)
			purchases.GET("/inquiries/:id/quotes", h.ListQuotes)
			purchases.POST("/quotes", middleware.RequireRole("admin", "manager"), h.CreateQuote)
			purchases.PUT("/quotes/:id/select", middleware.RequireRole("admin", "manager"), h.SelectQuote)
			purchases.GET("/orders", h.ListOrders)
			purchases.GET("/orders/:id", h.GetOrder)
			purchases.POST("/orders", middleware.RequireRole("admin", "manager"), h.CreateOrder)
			purchases.PUT("/orders/:id", middleware.RequireRole("admin", "manager"), h.UpdateOrder)
			purchases.POST("/orders/:id/transition", middleware.RequireRole("admin", "manager", "staff"), h.Transition)
			purchases.POST("/orders/:id/receive", middleware.RequireRole("admin", "manager", "staff"), h.Receive)
			purchases.GET("/orders/:id/logs", h.ListStatusLogs)
			purchases.GET("/receives", h.ListReceives)
			purchases.GET("/receives/:id", h.GetReceive)
		}

		// 库存管理
		inventory := authRequired.Group("/inventory")
		{
			h := handler.NewInventoryHandler(db)
			inventory.GET("", h.List)
			inventory.GET("/:id", h.Get)
			inventory.PUT("/:id", middleware.RequireRole("admin", "staff"), h.Update)
			inventory.POST("/adjust", middleware.RequireRole("admin", "staff"), h.Adjust)
			inventory.GET("/movements", h.ListMovements)
			inventory.GET("/alerts", h.ListAlerts)
			inventory.POST("/alerts/trigger", middleware.RequireRole("admin"), h.TriggerAlerts)
			inventory.POST("/alerts/:id/resolve", middleware.RequireRole("admin", "manager", "staff"), h.ResolveAlert)
			inventory.GET("/warehouses", h.ListWarehouses)
			inventory.POST("/warehouses", middleware.RequireRole("admin"), h.CreateWarehouse)
			inventory.PUT("/warehouses/:id", middleware.RequireRole("admin"), h.UpdateWarehouse)
		}

		// 对账与利润
		finance := authRequired.Group("/finance")
		{
			h := handler.NewFinanceHandler(db)
			finance.GET("/bills", h.ListBills)
			finance.GET("/bills/:id", h.GetBill)
			finance.POST("/bills", middleware.RequireRole("admin", "staff"), h.CreateBill)
			finance.POST("/bills/auto-match", middleware.RequireRole("admin", "staff"), h.AutoMatch)
			finance.PUT("/bills/:id", middleware.RequireRole("admin", "staff"), h.UpdateBill)
			finance.POST("/bills/:id/match", middleware.RequireRole("admin", "staff"), h.MatchBill)
			finance.POST("/bills/:id/pay", middleware.RequireRole("admin", "staff"), h.PayBill)
			finance.GET("/bills/:id/items", h.ListBillItems)
			finance.GET("/profit/summary", h.ProfitSummary)
			finance.GET("/profit/by-sku", h.ProfitBySKU)
			finance.GET("/profit/by-platform", h.ProfitByPlatform)
			finance.GET("/profit/trend", h.ProfitTrend)
		}

		// AI 工作流(gateway 单体模式也注册,本地处理)
		registerAIRoutes(authRequired, db)
		// RAG 知识库(gateway 单体模式也注册,本地处理)
		registerRAGRoutes(authRequired, db)

		// 数据看板
		dashboard := authRequired.Group("/dashboard")
		{
			h := handler.NewDashboardHandler(db)
			dashboard.GET("/overview", h.Overview)
			dashboard.GET("/sales-trend", h.SalesTrend)
			dashboard.GET("/category-share", h.CategoryShare)
			dashboard.GET("/product/stats", h.ProductStats)
			dashboard.GET("/purchase/stats", h.PurchaseStats)
			dashboard.GET("/inventory/stats", h.InventoryStats)
			dashboard.GET("/profit/stats", h.ProfitStats)
			dashboard.GET("/ai/stats", h.AIStats)
		}

		// 平台对接
		platform := authRequired.Group("/platform")
		{
			h := handler.NewPlatformHandler(db)
			platform.GET("/accounts", h.ListAccounts)
			platform.POST("/accounts", middleware.RequireRole("admin", "manager"), h.CreateAccount)
			platform.PUT("/accounts/:id", middleware.RequireRole("admin", "manager"), h.UpdateAccount)
			platform.POST("/accounts/:id/sync", middleware.RequireRole("admin", "manager", "staff"), h.SyncAccount)
			platform.GET("/accounts/:id/products", h.ListPlatformProducts)
			platform.GET("/accounts/:id/orders", h.ListPlatformOrders)
		}
	}
}

// registerAIRoutes 注册 AI 工作流相关路由(ai 角色和 gateway 角色)
// 包含: workflows / runs / prompts / analyze / generate / reply
func registerAIRoutes(g *gin.RouterGroup, db *gorm.DB) {
	aiGroup := g.Group("/ai")
	h := handler.NewAIHandler(db)
	aiGroup.GET("/workflows", h.ListWorkflows)
	aiGroup.GET("/workflows/:id", h.GetWorkflow)
	aiGroup.POST("/workflows", middleware.RequireRole("admin", "manager"), h.CreateWorkflow)
	aiGroup.PUT("/workflows/:id", middleware.RequireRole("admin", "manager"), h.UpdateWorkflow)
	aiGroup.POST("/workflows/:id/run", h.RunWorkflow)
	aiGroup.GET("/runs", h.ListRuns)
	aiGroup.GET("/runs/:id", h.GetRun)
	aiGroup.GET("/prompts", h.ListPrompts)
	aiGroup.POST("/prompts", middleware.RequireRole("admin", "manager"), h.CreatePrompt)
	aiGroup.PUT("/prompts/:id", middleware.RequireRole("admin", "manager"), h.UpdatePrompt)
	aiGroup.POST("/analyze/product", h.AnalyzeProduct)
	aiGroup.POST("/generate/listing", h.GenerateListing)
	aiGroup.POST("/reply/customer", h.ReplyCustomer)
}

// registerRAGRoutes 注册 RAG 知识库相关路由(rag 角色和 gateway 角色)
// 包含: knowledge-bases / rag/search
func registerRAGRoutes(g *gin.RouterGroup, db *gorm.DB) {
	aiGroup := g.Group("/ai")
	h := handler.NewAIHandler(db)
	aiGroup.GET("/knowledge-bases", h.ListKnowledgeBases)
	aiGroup.POST("/knowledge-bases", middleware.RequireRole("admin"), h.CreateKnowledgeBase)
	aiGroup.POST("/knowledge-bases/:id/documents", middleware.RequireRole("admin", "manager"), h.UploadDocument)
	aiGroup.POST("/knowledge-bases/:id/documents/upload", middleware.RequireRole("admin", "manager"), h.UploadDocumentFile)
	aiGroup.GET("/knowledge-bases/:id/documents", h.ListDocuments)
	aiGroup.POST("/rag/search", h.RAGSearch)
}
