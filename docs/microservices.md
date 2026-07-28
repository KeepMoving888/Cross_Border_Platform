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

### 目录结构(保持单仓库,单一 cmd 入口 + APP_ROLE 角色控制)

```
cb-platform/
├── cmd/
│   └── server/              # 唯一入口,通过 APP_ROLE 环境变量控制服务角色
├── internal/
│   ├── application/
│   │   ├── ai/              # AI 引擎(仅 gateway/ai 角色初始化)
│   │   ├── finance/         # 财务(仅 gateway 角色初始化)
│   │   ├── inventory/       # 库存(仅 gateway 角色初始化)
│   │   └── purchase/        # 采购(仅 gateway 角色初始化)
│   ├── interfaces/
│   │   └── http/
│   │       ├── server.go    # 路由注册(按 APP_ROLE 裁剪)
│   │       ├── proxy.go     # 反向代理中间件(仅 gateway 启用)
│   │       └── handler/     # 12 个 handler(共享,按角色注册)
│   ├── domain/models/       # 共享领域模型
│   └── pkg/                 # 共享基础设施
│       ├── config/          # 配置(ServiceConfig + App.Role)
│       └── ...
├── deployments/
│   ├── docker-compose.yml              # 单体版(APP_ROLE=gateway,默认)
│   └── docker-compose-microservices.yml # 微服务版(3 容器,各自 APP_ROLE)
```

### 拆分原则

1. **单一二进制多角色**:同一个 `cmd/server/main.go` 通过 `APP_ROLE` 环境变量控制初始化范围和路由注册,无需多 cmd 入口
2. **共享领域模型**:`domain/models` 保持共享,避免重复定义
3. **共享基础设施**:`pkg/config`、`pkg/database`、`pkg/logger` 保持共享
4. **共享 handler**:12 个 handler 保持共享,`registerRoutes` 按 `APP_ROLE` 选择性注册
5. **配置驱动**:`APP_ROLE=gateway|ai|rag` 控制服务角色,`AI_SERVICE_URL`/`RAG_SERVICE_URL` 控制反向代理

### APP_ROLE 角色裁剪矩阵

| 初始化项 | gateway | ai | rag |
|---|---|---|---|
| MySQL 连接 | ✅ | ✅ | ✅ |
| Redis 连接 | ✅ | ✅ | ✅ |
| PostgreSQL + pgvector | ❌ | ✅ | ✅ |
| AutoMigrate | ✅ | ❌ | ❌ |
| SeedData(开发模式) | ✅ | ❌ | ❌ |
| 对账自动匹配 | ✅ | ❌ | ❌ |
| 库存预警定时任务 | ✅ | ❌ | ❌ |
| AI 工作流调度器 | ✅ | ✅ | ❌ |
| 反向代理转发 | ✅(配置时) | ❌ | ❌ |
| 业务 CRUD 路由 | ✅ | ❌ | ❌ |
| AI 工作流路由 | ✅ | ✅ | ❌ |
| RAG 知识库路由 | ✅ | ❌ | ✅ |

## 四、实施路径(渐进式)

### Phase 1:反向代理 + APP_ROLE 角色裁剪(已实现)

**核心实现**:
- [proxy.go](../internal/interfaces/http/proxy.go):反向代理中间件,配置驱动转发
- [server.go](../internal/interfaces/http/server.go):`registerRoutes` 按 `APP_ROLE` 裁剪路由
- [main.go](../cmd/server/main.go):按 `APP_ROLE` 裁剪初始化范围(数据库/调度器/后台任务)

**工作原理**:

```go
// 1. main.go 根据 APP_ROLE 裁剪初始化
role := cfg.App.Role  // gateway(默认) / ai / rag
if role == RoleGateway {
    // 启动业务后台任务:对账、库存预警
    // 执行 AutoMigrate、SeedData
}
if role == RoleGateway || role == RoleAI {
    // 启动 AI 工作流调度器
}
// rag 角色只加载 PG + Embedder,不启动 AI 引擎

// 2. server.go 根据 APP_ROLE 裁剪路由
// gateway: 全部业务路由 + AI/RAG 路由(本地处理) + 反向代理(配置时)
// ai:      仅 auth + AI 工作流路由(workflows/runs/prompts/analyze/generate/reply)
// rag:     仅 auth + RAG 路由(knowledge-bases/rag/search)

// 3. 反向代理(仅 gateway 启用)
// 配置 AI_SERVICE_URL 后,/api/v1/ai/workflows 等转发到 AI Service
// 配置 RAG_SERVICE_URL 后,/api/v1/ai/knowledge-bases 等转发到 RAG Service
// 未配置时,所有路由本地处理(单体模式)
```

**优势**:
- 单一二进制,构建/部署流程统一
- 配置驱动,单体/微服务模式自由切换
- 资源隔离:ai 服务不加载业务模块,rag 服务不加载 AI 引擎
- 渐进式:无需拆分代码或仓库,通过环境变量即可切换

### Phase 2:独立仓库(可选,团队扩大后)

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
