# CB-Platform 微服务部署方案

## 一、现状分析

### 当前架构（单体）

```
cb-platform (单体)
├── cmd/server/main.go        # 唯一入口
├── internal/
│   ├── application/           # 业务逻辑(ai/finance/inventory/purchase)
│   ├── domain/models/         # 领域模型
│   ├── interfaces/http/       # HTTP handler(12 个)
│   └── pkg/                   # 基础设施(config/database/middleware)
└── web/                       # 前端(独立部署)
```

**单体优势**：开发简单、调试方便、部署成本低
**单体瓶颈**：AI 工作流计算密集、RAG 向量检索资源隔离需求、水平扩展粒度粗

### 微服务拆分判断

| 判断维度 | 当前状态 | 是否需要拆分 |
|---|---|---|
| 团队规模 | 单团队 | 否（< 5 人不建议拆） |
| 部署频率 | 统一发布 | 否 |
| 资源隔离 | AI/RAG 计算影响主业务 | **是** |
| 独立扩展 | AI 工作流需独立水平扩展 | **是** |
| 故障隔离 | AI 服务崩溃影响全站 | **是** |

**结论**：当前阶段不需要全量微服务化，但建议**按资源特性拆分 3 个服务**（API 网关 + AI 服务 + RAG 服务），实现计算资源隔离和独立扩展。

## 二、目标架构（3 服务 + 基础设施）

```
                    ┌─────────────────┐
                    │   Nginx / CDN   │  ← 前端静态资源
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │  API Gateway    │  ← 8080 端口
                    │  (cb-platform)  │     业务 CRUD + 认证
                    └───┬────┬────┬───┘
                        │    │    │
          ┌─────────────┘    │    └─────────────┐
          │                  │                  │
┌─────────▼──────┐  ┌────────▼────────┐  ┌──────▼─────────┐
│  AI Service    │  │  RAG Service    │  │  (未来可扩展)  │
│  (cb-ai-svc)   │  │  (cb-rag-svc)   │  │  Text2SQL 服务 │
│  8081          │  │  8082           │  │  8083          │
│                │  │                 │  │                │
│  - 工作流执行  │  │  - 向量检索     │  │  - NL2SQL      │
│  - LLM 调用    │  │  - 文档入库     │  │  - SQL 执行    │
│  - 调度器      │  │  - Reranker     │  │  - 洞察生成    │
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

### 服务职责

| 服务 | 端口 | 职责 | 资源特性 |
|---|---|---|---|
| API Gateway (cb-platform) | 8080 | 业务 CRUD、认证鉴权、消息、采购、库存、财务 | IO 密集，低 CPU |
| AI Service (cb-ai-svc) | 8081 | AI 工作流执行、LLM 调用、调度器、Text2SQL | CPU 密集，长连接 |
| RAG Service (cb-rag-svc) | 8082 | 向量检索、文档入库、Reranker、Embedding | 内存密集，GPU 可选 |

### 通信方式

| 场景 | 方式 | 说明 |
|---|---|---|
| 前端 → API Gateway | HTTP | 标准 REST |
| API Gateway → AI/RAG Service | HTTP (内部) | 简单直接，无需 gRPC |
| AI Service → RAG Service | HTTP (内部) | 工作流 RAG 节点调用 |
| 服务 → MySQL/Redis/PG | TCP | 直连 |
| 服务 → Prometheus | HTTP pull | 指标采集 |

## 三、代码拆分方案

### 目录结构（保持单仓库，多 cmd 入口）

```
cb-platform/
├── cmd/
│   ├── server/          # API Gateway 入口（现有，精简）
│   ├── ai-service/      # AI 服务入口（新增）
│   └── rag-service/     # RAG 服务入口（新增）
├── internal/
│   ├── application/
│   │   ├── ai/          # AI 引擎（移至 ai-service）
│   │   ├── finance/     # 财务（留 API Gateway）
│   │   ├── inventory/   # 库存（留 API Gateway）
│   │   └── purchase/    # 采购（留 API Gateway）
│   ├── interfaces/
│   │   └── http/
│   │       ├── gateway/     # API Gateway 路由（业务）
│   │       ├── ai-handler/  # AI 服务路由
│   │       └── rag-handler/ # RAG 服务路由
│   ├── domain/models/   # 共享领域模型
│   └── pkg/             # 共享基础设施
├── deployments/
│   ├── docker-compose.yml              # 单体版（现有）
│   └── docker-compose-microservices.yml # 微服务版（新增）
```

### 拆分原则

1. **共享领域模型**：`domain/models` 保持共享，避免重复定义
2. **共享基础设施**：`pkg/config`、`pkg/database`、`pkg/logger` 保持共享
3. **独立 cmd 入口**：每个服务有独立的 `main.go`，按需初始化依赖
4. **配置驱动**：通过环境变量 `APP_ROLE=gateway|ai|rag` 控制服务角色

## 四、实施路径（渐进式）

### Phase 1：多 cmd 入口（当前，无需改代码）

当前 `cmd/server/main.go` 已支持通过 flag 控制行为。Phase 1 通过**同一二进制 + 环境变量**模拟多服务：

```bash
# API Gateway（业务 + 转发 AI/RAG 请求）
APP_ROLE=gateway ./cb-platform

# AI Service（工作流 + LLM）
APP_ROLE=ai ./cb-platform

# RAG Service（向量检索 + 文档入库）
APP_ROLE=rag ./cb-platform
```

优势：零代码改动，验证部署架构和资源隔离效果。

### Phase 2：独立 cmd 入口（代码拆分）

创建独立的 `cmd/ai-service/main.go` 和 `cmd/rag-service/main.go`，各自只初始化所需依赖。

### Phase 3：独立仓库（可选，团队扩大后）

当团队 > 5 人时，拆分为独立仓库：
- `cb-platform-core`：API Gateway
- `cb-platform-ai`：AI 服务
- `cb-platform-rag`：RAG 服务
- `cb-platform-shared`：共享 SDK（模型、配置）

## 五、API Gateway 转发规则

API Gateway 作为唯一入口，内部服务对前端透明：

| 路径前缀 | 转发目标 | 说明 |
|---|---|---|
| `/api/v1/ai/workflows` | AI Service | 工作流管理 + 执行 |
| `/api/v1/ai/runs` | AI Service | 执行历史 |
| `/api/v1/ai/prompts` | AI Service | Prompt 模板 |
| `/api/v1/knowledge-bases` | RAG Service | 知识库管理 |
| `/api/v1/knowledge-bases/{id}/documents` | RAG Service | 文档上传 |
| `/api/v1/knowledge-bases/{id}/search` | RAG Service | RAG 检索 |
| 其他 | 本地处理 | 业务 CRUD |

## 六、部署配置

使用 `deployments/docker-compose-microservices.yml` 启动微服务版：

```bash
cd deployments
docker compose -f docker-compose-microservices.yml up -d
```

### 资源限制建议

| 服务 | CPU | 内存 | 副本数 |
|---|---|---|---|
| API Gateway | 0.5 | 512MB | 2 |
| AI Service | 2.0 | 2GB | 2 |
| RAG Service | 1.0 | 1GB | 1-3 |
| MySQL | 1.0 | 1GB | 1 |
| PostgreSQL | 1.0 | 1GB | 1 |
| Redis | 0.5 | 256MB | 1 |
| Milvus（可选） | 2.0 | 4GB | 1 |

## 七、监控与可观测性

每个服务独立暴露 `/metrics` 端点，Prometheus 自动采集：

| 服务 | 关键指标 |
|---|---|
| API Gateway | http_requests_total, http_request_duration_seconds |
| AI Service | ai_workflow_runs_total, ai_workflow_duration_seconds |
| RAG Service | rag_search_duration_seconds, rag_search_score, rag_fallback_total |

Grafana 仪表盘通过 `job` 标签区分服务，已支持多服务场景。

## 八、适用场景判断

| 场景 | 推荐架构 |
|---|---|
| 个人项目 / Demo | 单体（docker-compose.yml） |
| 小团队 / < 100 用户 | 单体 |
| 中型团队 / AI 负载重 | **微服务（docker-compose-microservices.yml）** |
| 大规模 / 多团队 | 微服务 + K8s |
