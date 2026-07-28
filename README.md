# CB-Platform 跨境电商智能运营中台

![Dashboard](docs/images/dashboard-preview.jpg)

## 核心价值

**AI 驱动的跨境电商全链路运营平台**，覆盖「选品 → 采购 → 库存 → 财务 → AI 决策」完整业务闭环，面向家电美容赛道（个护电器 + 美容仪器 + 美体仪器 + 配件耗材）。

### 差异化能力

| 能力 | 传统 ERP | CB-Platform |
|---|---|---|
| 选品决策 | 人工经验判断 | AI 工作流多维度评分（毛利率/月销/评分/趋势/竞争度加权），输出推荐结论+风险提示 |
| 采购谈判 | 人工比价 | AI 谈判助手一键分析报价合理性、市场均价、交付风险、检查清单 |
| 客户服务 | 人工逐条回复 | AI 客服工作流自动生成多语言回复，意图识别+置信度+建议动作 |
| 数据分析 | BI 报表固定维度 | 自然语言转 SQL（Text2SQL），输入问题即返回 SQL+查询结果+业务洞察 |
| 工作流编排 | 无 | React Flow 可视化拖拽编排，7 种节点类型（input/llm/rag/text2sql/sql_execute/condition/output） |

## 技术架构

```
┌─────────────────────────────────────────────────────┐
│  Frontend (React 18 + TypeScript + Ant Design v5)  │
│  Vite + ECharts + React Flow + Zustand             │
├─────────────────────────────────────────────────────┤
│  Backend (Go 1.22 + Gin + GORM)                    │
│  ┌──────────┐ ┌──────────┐ ┌─────────────────────┐ │
│  │ REST API │ │ AI 引擎  │ │ Prometheus 指标     │ │
│  │ 8 模块   │ │ 7 节点   │ │ 4 项 AI 专用指标    │ │
│  └──────────┘ └──────────┘ └─────────────────────┘ │
├─────────────────────────────────────────────────────┤
│  MySQL │ Redis │ PostgreSQL(pgvector) │ Milvus     │
└─────────────────────────────────────────────────────┘
```

### 后端工程

- **领域驱动设计**：`interfaces/http` → `application` → `domain/models` → `pkg/database` 四层架构
- **AI 工作流引擎**：DAG 拓扑执行，支持 input → llm/rag/text2sql/sql_execute/condition → output 节点链路
- **LLM Provider 抽象**：GLM / Claude / DeepSeek / Qwen / OpenAI 兼容协议，API Key 为空时自动回退 BuiltinLLMProvider（基于场景识别的结构化输出）
- **Text2SQL**：自然语言 → SQL 生成 → SELECT 安全校验 → 执行 → 业务洞察生成
- **RAG 检索**：pgvector 向量检索（余弦相似度）+ BM25 全文检索（PostgreSQL tsvector）+ TF-IDF 降级，三路混合检索 + RRF 融合（k=60）+ Reranker 重排序（cross-encoder API / 启发式降级）；Redis 检索结果缓存（FNV-1a hash key，TTL 1h）
- **VectorStore 抽象层**：`VectorStore` 接口解耦 RAGService 与向量存储实现，3 种可插拔实现：`PgVectorStore`（生产，批量 UPDATE...FROM VALUES 优化）、`MilvusStore`（REST API，10M+ 大规模向量，IVF/HNSW 索引）、`InMemoryVectorStore`（测试/本地开发，暴力余弦相似度）；`NewVectorStore` 工厂函数按配置自动选择
- **批量入库**：`BatchIndexDocuments` 跨文档批量入库，合并所有 chunks 一次性调用 Embedding API + 单次 UpsertVectors 批量写入，显著降低 API 调用次数和 RTT
- **微服务部署**：支持单体（`docker-compose.yml`）和微服务（`docker-compose-microservices.yml`）两种部署模式，微服务版拆分 API Gateway + AI Service + RAG Service 3 个服务，实现资源隔离和独立扩展
- **RAG 多模态**：支持 PDF / Word(.docx) / Markdown / TXT 文件上传，自动解析为纯文本后分块入库（DocxParser 解析 XML、PDFParser 提取文本流）
- **定时调度器**：后台 goroutine 每分钟扫描启用的工作流，支持 scheduled 触发
- **Prometheus 指标**：`ai_workflow_runs_total` / `ai_workflow_duration_seconds` / `ai_workflow_tokens_total` / `ai_workflow_cost_usd`；RAG 专用 `rag_search_duration_seconds` / `rag_search_score` / `rag_search_total` / `rag_fallback_total` / `rag_cache_hits_total` / `rag_rerank_duration_seconds` / `rag_rerank_total` / `rag_index_docs_total`

### 前端工程

- **13 个业务页面**：工作台、选品列表、选品详情、采购管理、库存管理、财务管理、AI 工作流、工作流历史、知识库、客服消息、工作流编排、平台管理、登录
- **AI 决策卡**：ProductDetail 页点击「AI 深度分析」调用后端工作流，返回评分+推荐结论+理由+风险，本地算法兜底
- **AI 数据分析**：Dashboard 内嵌自然语言查询框，快捷问题 Tag，结果含 SQL+动态列表格+业务洞察
- **结构化结果展示**：5 种场景差异化渲染（评分仪表盘/报价对比/客服回复/SQL 结果表/Listing 生成）
- **工作流可视化编排**：React Flow 拖拽节点+连线+属性配置+JSON 导出
- **响应式设计**：移动端/桌面端断点适配，数据表格横向滚动

## 业务模块

| 模块 | 核心功能 |
|---|---|
| 选品管理 | AI 评分排行、选品漏斗、趋势图、竞品雷达、成本结构、AI 决策建议卡 |
| 采购管理 | 9 状态机流转（询价→报价→下单→跟踪→发货→收货→质检→对账→结算）、AI 谈判助手 |
| 库存管理 | 多仓覆盖（深圳主仓/美西/欧洲/FBA）、安全库存预警、库存变动流水 |
| 财务管理 | 账单对账、利润报表（每日 SKU 级收入/成本/净利润）、趋势分析 |
| AI 工作流 | 5 个预置场景、工作流历史记录、趋势图、Token/成本统计 |
| 知识库 | RAG 混合检索(向量+BM25+RRF)、Reranker 重排序、多模态文档上传(PDF/Word/MD)、状态轮询、检索测试与相似度可视化 |
| 客服消息 | 多平台消息聚合、AI 回复建议、意图识别、多语言支持 |
| 工作流编排 | 可视化拖拽编排、7 种节点类型、JSON 导出/导入 |

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.22、Gin、GORM、MySQL 8、Redis 7、PostgreSQL(pgvector)、Milvus |
| 前端 | React 18、TypeScript 5、Ant Design v5、Vite 5、ECharts 5、React Flow 11、Zustand |
| AI | GLM-4-Plus / Claude / DeepSeek / Qwen（Provider 抽象，无 Key 时 BuiltinLLMProvider） |
| 运维 | Docker Compose、Prometheus、Grafana、结构化日志 |

## 快速启动

```bash
# 方式一:单体部署(默认,适合开发和小规模)
cd deployments
docker compose up -d

# 方式二:微服务部署(适合 AI 负载重、需资源隔离)
docker compose -f docker-compose-microservices.yml up -d

# 启动前端
cd web
pnpm install
pnpm dev

# 访问
# 前端: http://localhost:5174
# 后端: http://localhost:8088
# 默认账号: admin / admin123
```

> 微服务部署详见 [docs/microservices.md](docs/microservices.md)，包含架构设计、服务拆分、资源限制和渐进式实施路径。

## 项目结构

```
cb-platform/
├── cmd/server/              # 后端入口
├── internal/
│   ├── application/         # 业务逻辑层
│   │   ├── ai/             # AI 工作流引擎(7 节点类型 + 调度器 + RAG)
│   │   ├── finance/        # 财务服务
│   │   ├── inventory/      # 库存服务
│   │   └── purchase/       # 采购状态机
│   ├── domain/models/      # 领域模型(15+ 实体)
│   ├── interfaces/http/    # HTTP 接口层(12 个 handler)
│   └── pkg/                # 基础设施(config/database/middleware/logger/response)
├── web/                     # 前端(13 个页面)
│   └── src/
│       ├── pages/          # 业务页面
│       ├── api/            # API 封装
│       ├── components/     # 通用组件
│       ├── store/          # Zustand 状态管理
│       └── types/          # TypeScript 类型定义
├── deployments/             # Docker Compose 部署
└── docs/                    # 文档与截图
```

## License

本项目采用 **CC BY-NC 4.0** (Creative Commons Attribution-NonCommercial 4.0 International) 协议开源。

- ✅ 允许：学习、研究、个人使用、二次开发（需署名）
- ❌ 禁止：任何形式的商业用途（销售、SaaS 服务、内部商业运营等）

详细条款见 [LICENSE](LICENSE) 文件。如需商业授权，请联系仓库所有者。
