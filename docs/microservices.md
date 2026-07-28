# CB-Platform 微服务部署方案

## 一、现状分析

### 当前架构（单体）

```
cb-platform (单体)
├── cmd/server/main.go              # 唯一入口(package main,Go 主函数)
├── internal/
│   ├── application/                # 业务逻辑
│   │   ├── ai/                     # AI 引擎(工作流+RAG+调度器)
│   │   ├── finance/                # 财务对账
│   │   ├── inventory/              # 库存预警
│   │   └── purchase/               # 采购状态机
│   ├── domain/models/              # 领域模型(15+ 实体)
│   ├── interfaces/http/            # HTTP 接口层
│   │   ├── server.go               # 路由注册(12 个 handler 组)
│   │   └── handler/                # 12 个 handler(ai/auth/dashboard/finance/...)
│   └── pkg/                        # 基础设施(config/database/middleware/logger)
└── web/                            # 前端(独立部署,13 个页面)
```

### 实际路由分组(来自 server.go)

| 路由前缀 | handler | 业务域 | 资源特性 |
|---|---|---|---|
| `/api/v1/auth` | Auth | 认证 | IO 密集 |
| `/api/v1/users` | User | 用户管理 | IO 密集 |
| `/api/v1/messages` | Message | 消息中心 | IO 密集 |
| `/api/v1/suppliers` | Supplier | 供应商 | IO 密集 |
| `/api/v1/products` | Product | 选品管理 | IO 密集 |
| `/api/v1/purchases` | Purchase | 采购管理 | IO 密集 |
| `/api/v1/inventory` | Inventory | 库存管理 | IO 密集 |
| `/api/v1/finance` | Finance | 财务对账 | IO 密集 |
| `/api/v1/ai/workflows` | AI | **AI 工作流执行** | **CPU 密集** |
| `/api/v1/ai/runs` | AI | 执行历史 | IO 密集 |
| `/api/v1/ai/prompts` | AI | Prompt 模板 | IO 密集 |
| `/api/v1/ai/knowledge-bases` | AI | **RAG 知识库** | **内存密集** |
| `/api/v1/ai/rag/search` | AI | **向量检索** | **内存密集** |
| `/api/v1/dashboard` | Dashboard | 数据看板 | IO 密集 |
| `/api/v1/platform` | Platform | 平台对接 | IO 密集 |

### 微服务拆分判断

| 判断维度 | 当前状态 | 是否需要拆分 |
|---|---|---|
| 团队规模 | 单团队 | 否(< 5 人不建议拆) |
| 部署频率 | 统一发布 | 否 |
| 资源隔离 | AI 工作流 CPU 密集、RAG 内存密集,影响主业务 CRUD | **是** |
| 独立扩展 | AI 工作流并发受 LLM 限流,需独立水平扩展 | **是** |
| 故障隔离 | AI 服务崩溃或 LLM 超时影响全站可用性 | **是** |
| 数据一致性 | 跨服务事务需求弱(业务与 AI 无强一致需求) | **是** |

**结论**:当前阶段不需要全量微服务化,但建议**按资源特性拆分 3 个服务**(API Gateway + AI Service + RAG Service),实现计算资源隔离和独立扩展。

## 二、目标架构(3 服务 + 基础设施)

```
                    ┌─────────────────┐
                    │   Nginx / CDN   │  ← 前端静态资源(web/dist)
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │  API Gateway    │  ← 8080 端口
                    │  (cb-platform)  │     业务 CRUD + 认证 + 请求转发
                    └───┬────┬────┬───┘
                        │    │    │
          ┌─────────────┘    │    └─────────────┐
          │                  │                  │
┌─────────▼──────┐  ┌────────▼────────┐  ┌──────▼─────────┐
│  AI Service    │  │  RAG Service    │  │  (未来可扩展)  │
│  (cb-ai-svc)   │  │  (cb-rag-svc)   │  │  Text2SQL 服务 │
│  8081          │  │  8082           │  │  8083          │
│                │  │                 │  │                │
│  工作流执行    │  │  向量检索       │  │  NL2SQL        │
│  LLM 调用      │  │  文档入库       │  │  SQL 执行      │
│  调度器        │  │  Reranker       │  │  洞察生成      │
│  Text2SQL      │  │  Embedding      │  │                │
└────────────────┘  └─────────────────┘  └────────────────┘
          │                  │                  │
          └──────────┬───────┴──────────────────┘
                     │
    ┌────────────────┼────────────────┐
    │                │                │
┌───▼───┐    ┌───────▼───────┐  ┌─────▼─────┐
│ MySQL │    │ PostgreSQL    │  │  Milvus   │
│ 业务  │    │ + pgvector   │  │  (可选)   │
└───────┘    └───────────────┘  └───────────┘
                     │                │
              ┌──────▼──────┐  ┌──────▼──────┐
              │   Redis     │  │ Prometheus  │
              │   缓存      │  │ + Grafana   │
              └─────────────┘  └─────────────┘
```

### 服务职责(基于实际路由)

| 服务 | 端口 | 承接路由 | 资源特性 |
|---|---|---|---|
| API Gateway (cb-platform) | 8080 | auth/users/messages/suppliers/products/purchases/inventory/finance/dashboard/platform + **请求转发** | IO 密集,低 CPU |
| AI Service (cb-ai-svc) | 8081 | `/api/v1/ai/workflows` + `/api/v1/ai/runs` + `/api/v1/ai/prompts` + `/api/v1/ai/analyze/*` + `/api/v1/ai/generate/*` + `/api/v1/ai/reply/*` | CPU 密集,长连接(LLM 流式) |
| RAG Service (cb-rag-svc) | 8082 | `/api/v1/ai/knowledge-bases` + `/api/v1/ai/rag/search` | 内存密集(向量计算) |

### API Gateway 转发规则(基于 server.go 实际路由)

| 路径前缀 | 转发目标 | 转发方式 | 说明 |
|---|---|---|---|
| `/api/v1/ai/workflows` | AI Service | 反向代理 | 工作流管理 + 执行 |
| `/api/v1/ai/runs` | AI Service | 反向代理 | 执行历史 |
| `/api/v1/ai/prompts` | AI Service | 反向代理 | Prompt 模板 |
| `/api/v1/ai/analyze` | AI Service | 反向代理 | 选品分析 |
| `/api/v1/ai/generate` | AI Service | 反向代理 | Listing 生成 |
| `/api/v1/ai/reply` | AI Service | 反向代理 | 客服回复 |
| `/api/v1/ai/knowledge-bases` | RAG Service | 反向代理 | 知识库管理 + 文档上传 |
| `/api/v1/ai/rag` | RAG Service | 反向代理 | RAG 检索 |
| 其他 `/api/v1/*` | 本地处理 | 直连 | 业务 CRUD |

**注意**:AI Service 与 RAG Service 路由前缀均为 `/api/v1/ai/*`,Gateway 需按子路径精确匹配转发。

### 通信方式

| 场景 | 方式 | 说明 |
|---|---|---|
| 前端 → API Gateway | HTTP | 标准 REST |
| API Gateway → AI/RAG Service | HTTP 反向代理 | 透明转发,JWT 透传 |
| AI Service → RAG Service | HTTP(内部) | 工作流 RAG 节点调用 RAG 检索接口 |
| 服务 → MySQL/Redis/PG | TCP | 直连(共享数据库) |
| 服务 → Prometheus | HTTP pull | 指标采集(`/metrics` 端点) |

## 三、代码拆分方案

### 目录结构(保持单仓库,多 cmd 入口)

```
cb-platform/
├── cmd/
│   ├── server/          # API Gateway 入口(现有,精简为业务+转发)
│   ├── ai-service/      # AI 服务入口(Phase 2 新增)
│   └── rag-service/     # RAG 服务入口(Phase 2 新增)
├── internal/
│   ├── application/
│   │   ├── ai/          # AI 引擎(Phase 2 移至 ai-service)
│   │   ├── finance/     # 财务(留 API Gateway)
│   │   ├── inventory/   # 库存(留 API Gateway)
│   │   └── purchase/    # 采购(留 API Gateway)
│   ├── interfaces/
│   │   └── http/
│   │       ├── server.go        # 路由注册(支持按角色裁剪)
│   │       ├── proxy.go         # 反向代理中间件(已实现)
│   │       └── handler/         # 12 个 handler(共享)
│   ├── domain/models/   # 共享领域模型
│   └── pkg/             # 共享基础设施
│       ├── config/      # 配置(新增 ServiceConfig 微服务配置)
│       └── ...
├── deployments/
│   ├── docker-compose.yml              # 单体版(现有)
│   └── docker-compose-microservices.yml # 微服务版(已实现)
```

### 拆分原则

1. **共享领域模型**:`domain/models` 保持共享,避免重复定义
2. **共享基础设施**:`pkg/config`、`pkg/database`、`pkg/logger` 保持共享
3. **共享 handler**:12 个 handler 保持共享,各服务按需注册路由
4. **独立 cmd 入口**:每个服务有独立的 `main.go`,按需初始化依赖
5. **配置驱动**:通过环境变量 `APP_ROLE=gateway|ai|rag` 控制服务角色

## 四、实施路径(渐进式)

### Phase 1:反向代理转发(已实现,零业务代码改动)

在 `server.go` 的 `NewRouter` 中注入 `ProxyConfig`,当配置了 `AI_SERVICE_URL` / `RAG_SERVICE_URL` 时自动启用反向代理转发。

**核心实现**:[proxy.go](../internal/interfaces/http/proxy.go)

```go
// 当配置了 AI_SERVICE_URL 时,/api/v1/ai/workflows 等路径转发到 AI Service
// 当配置了 RAG_SERVICE_URL 时,/api/v1/ai/knowledge-bases 等路径转发到 RAG Service
// 未配置时,所有路由本地处理(单体模式)
proxy := NewProxyMiddleware(cfg.Service)
proxy.Register(r)
```

**优势**:零业务代码改动,单体和微服务模式自由切换。

### Phase 2:独立 cmd 入口(代码拆分)

创建独立的 `cmd/ai-service/main.go` 和 `cmd/rag-service/main.go`,各自只初始化所需依赖:

- `ai-service`:只加载 AI 引擎 + LLM + 调度器,不初始化财务/库存/采购服务
- `rag-service`:只加载 RAG 服务 + VectorStore + Embedder,不初始化 AI 工作流

### Phase 3:独立仓库(可选,团队扩大后)

当团队 > 5 人时,拆分为独立仓库:
- `cb-platform-core`:API Gateway
- `cb-platform-ai`:AI 服务
- `cb-platform-rag`:RAG 服务
- `cb-platform-shared`:共享 SDK(模型、配置)

## 五、部署配置

使用 `deployments/docker-compose-microservices.yml` 启动微服务版:

```bash
cd deployments
docker compose -f docker-compose-microservices.yml up -d

# 启用 Milvus 大规模向量库(可选)
docker compose -f docker-compose-microservices.yml --profile milvus up -d
```

### 资源限制建议

| 服务 | CPU | 内存 | 副本数 | 扩展依据 |
|---|---|---|---|---|
| API Gateway | 0.5 | 512MB | 2 | QPS |
| AI Service | 2.0 | 2GB | 2-4 | LLM 并发数 |
| RAG Service | 1.0 | 1GB | 1-3 | 检索 QPS |
| MySQL | 1.0 | 1GB | 1(主从) | 数据量 |
| PostgreSQL | 1.0 | 1GB | 1 | 向量规模 |
| Redis | 0.5 | 256MB | 1 | 缓存命中率 |
| Milvus(可选) | 2.0 | 4GB | 1 | 向量规模 > 1M |

## 六、监控与可观测性

每个服务独立暴露 `/metrics` 端点,Prometheus 自动采集:

| 服务 | 关键指标 | 告警阈值 |
|---|---|---|
| API Gateway | `http_request_duration_seconds`(P95) | > 1s |
| AI Service | `ai_workflow_duration_seconds`(P95) | > 30s |
| AI Service | `ai_workflow_runs_total{status="failed"}` | 5min 内 > 10 |
| RAG Service | `rag_search_duration_seconds`(P95) | > 2s |
| RAG Service | `rag_fallback_total` | 5min 内 > 50 |
| RAG Service | `rag_search_score`(P50) | < 0.5 |

Grafana 仪表盘通过 `job` 标签区分服务,已支持多服务场景。

## 七、适用场景判断

| 场景 | 推荐架构 | 启动命令 |
|---|---|---|
| 个人项目 / Demo | 单体 | `docker compose up -d` |
| 小团队 / < 100 用户 | 单体 | `docker compose up -d` |
| 中型团队 / AI 负载重 | **微服务** | `docker compose -f docker-compose-microservices.yml up -d` |
| 大规模 / 多团队 | 微服务 + K8s | Helm Chart(Phase 3) |

## 八、服务发现与容错

### 服务发现(当前:Docker DNS)

微服务版通过 Docker Compose 内置 DNS 实现服务发现:
- `cb-ai-svc` 容器名即域名,Gateway 通过 `http://cb-ai-svc:8081` 访问
- `cb-rag-svc` 同理,通过 `http://cb-rag-svc:8082` 访问

### 容错策略

| 故障场景 | 处理策略 | 实现 |
|---|---|---|
| AI Service 不可用 | Gateway 返回 503 + 友好错误信息 | 反向代理超时 5s + fallback |
| RAG Service 不可用 | AI 工作流 RAG 节点降级到 TF-IDF | RAGService 内置降级 |
| LLM API 超时 | AI 工作流返回 BuiltinLLMProvider 结果 | LLM Provider 抽象 |
| PostgreSQL 不可用 | RAG 降级到 MySQL TF-IDF 检索 | RAGService 降级链路 |
| Redis 不可用 | 缓存穿透到数据库,不影响功能 | GetRedisSafe() |

### 健康检查

每个服务暴露 `/health` 端点,Docker Compose healthcheck 自动探测:

```json
{
  "status": "ok",
  "service": "cb-platform",
  "db": { "mysql": "ok", "redis": "ok", "postgres": "ok" }
}
```
