# 变更日志

本项目遵循 [Semantic Versioning](https://semver.org/lang/zh-CN/) 与 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/) 规范。

## [Unreleased]

### 计划中
- 平台 API 适配器接入真实环境(亚马逊 SP-API / Temu / TikTok Shop)
- RAG 节点向量检索(基于 pgvector 替换关键词检索)
- 工作流可视化编排前端
- WebSocket 实时通知(对账差异、库存预警)
- 多租户隔离能力

## [1.0.0] - 2026-07-26

### 新增

#### 核心业务模块
- **选品管理**:选品池、阶段流转(待评估-调研中-测试中-已上架-已淘汰)、AI 评分、市场趋势记录、竞品监控
- **采购管理**:基于 `looplab/fsm` 的状态机驱动,覆盖 询价 → 比价 → 下单 → 发货 → 入库 → 质检 → 对账 → 结算 全流程,支持取消与重开
- **库存管理**:多仓库、可用/锁定/在途三类库存、原子调整、流水审计、安全库存预警
- **对账与利润**:供应商账单、对账差异检测、多维度利润核算(收入/成本/费用三层拆解)
- **供应商管理**:资质档案、评级、合作状态、报价历史
- **数据看板**:6 个统计接口(总览、选品、采购、库存、利润、对账)

#### AI 工作流引擎
- 自研轻量节点编排引擎,支持 8 类节点:input / output / llm / rag / condition / text2sql / sql_execute / tool
- 5 个开箱即用工作流:
  - `wf_product_analysis` AI 选品分析师(agent 类型)
  - `wf_purchase_assistant` AI 采购助手(rag 类型)
  - `wf_customer_service` AI 客服回复(rag 类型)
  - `wf_content_generation` AI 内容生成(automation 类型)
  - `wf_data_analysis` AI 数据分析(text2sql 类型)
- 多 LLM Provider 抽象:GLM / Claude / DeepSeek / Qwen / OpenAI 兼容协议,运行时切换
- Builtin Provider 兜底:无 API Key 时仍可联调全流程
- 执行审计:每次运行记录输入、输出、Token 消耗、成本估算、耗时

#### 平台对接
- 平台适配器抽象层 `PlatformAdapter`,统一亚马逊 / Temu / TikTok Shop 接入方式
- 内置 Provider 实现预置数据,支持同步商品、订单、库存
- 平台账户管理 HTTP 接口(增删改查、数据同步)
- 真实 API 接入预留(配置 `RefreshToken` 即可启用)

#### 工程基础设施
- **DDD 分层架构**:domain / application / infrastructure / interfaces 四层清晰分离
- **配置管理**:Viper 多环境(.env + 环境变量),覆盖 MySQL/PG/Redis/JWT/LLM/Log
- **日志系统**:Zap 结构化日志,三路输出(控制台 + app.log + error.log),带 trace_id
- **错误处理**:统一错误码段(1xxx 参数 / 2xxx 认证 / 4xxx 资源 / 5xxx 状态 / 9xxx 系统),自动 HTTP 状态映射
- **响应封装**:统一 `{code, message, data, trace_id}` 格式,支持分页 `PageResult`
- **中间件链**:TraceID → Recovery → Logger → Metrics → CORS → Auth → RateLimiter
- **数据库迁移**:GORM AutoMigrate + SQL 迁移文件(`migrations/001_init.sql`)
- **种子数据**:内置用户、供应商、选品、采购单等业务数据,开箱可用

#### 可观测性
- Prometheus 指标采集:`http_requests_total`、`http_request_duration_seconds`、`http_requests_in_flight`、`http_response_size_bytes`
- Grafana 可视化:预配置数据源与 Dashboard
- 链路追踪:每请求生成 `trace_id`,Header 透传,日志关联
- 健康检查:`/health`(应用 + DB 状态)、`/ping`(轻量存活)

#### 部署与运维
- Docker Compose 一键部署(应用 + MySQL + Redis + PostgreSQL/pgvector + Prometheus + Grafana)
- 多阶段构建 Dockerfile,镜像精简
- 优雅启停:支持 SIGINT/SIGTERM,10 秒连接排水
- Makefile 工程化命令:build / run / test / lint / fmt / migrate / seed / docker-up / release
- CI/CD:GitHub Actions 自动化测试、代码检查、安全扫描(govulncheck)

#### 测试
- 单元测试覆盖核心包:errors / response / config / middleware / helpers
- 领域测试:采购状态机合法/非法转换、平台适配器
- AI 引擎测试:工作流执行、节点工厂、条件分支
- 测试风格:table-driven,标准库断言

#### 负载测试
- Go 原生压测工具 `scripts/loadtest/loadtest.go`:令牌桶控速、并发池、P50/P90/P95/P99 统计
- k6 脚本 `scripts/loadtest/k6_loadtest.js`:阶段性爬升、阈值断言、Prometheus 输出

#### 文档
- README:项目定位、快速开始、API 总览、技术栈
- 架构文档 `docs/architecture.md`:分层设计、状态机、AI 引擎、数据模型、可观测性
- API 文档 `docs/api.md`:完整接口列表与示例
- 部署文档 `docs/deployment.md`:生产环境部署、资源规划、安全加固、数据备份
- 贡献指南 `CONTRIBUTING.md`:开发环境、代码规范、测试要求、提交规范、PR 流程
- 变更日志 `CHANGELOG.md`

### 技术指标

| 维度 | 指标 |
|---|---|
| Go 版本 | 1.22 |
| 代码行数 | ~8000 LOC(含测试) |
| 业务表 | 18 张 |
| HTTP 接口 | 40+ |
| AI 工作流 | 5 个开箱即用 |
| 单元测试覆盖 | 核心包 ≥ 85% |
| 状态机 | 10 状态 / 11 事件 |
| LLM Provider | 5 + Builtin 兜底 |
| 容器服务 | 6 个(应用 + MySQL + Redis + PG + Prometheus + Grafana) |

### 性能基准

- **目标 QPS**:健康检查 ≥ 5000、业务接口 ≥ 500
- **目标延迟**:P95 < 500ms、P99 < 1000ms
- **错误率**:压测期间 < 0.1%
- 详见 `docs/benchmark.md`

## 版本规则

- `MAJOR`:不兼容的架构变更
- `MINOR`:向下兼容的功能新增
- `PATCH`:向下兼容的问题修复

## 发布流程

1. 更新本文件 `CHANGELOG.md`
2. 创建 git tag:`git tag -a v1.0.0 -m "Release v1.0.0"`
3. 推送 tag:`git push origin v1.0.0`
4. CI 自动构建 Docker 镜像并发布 GitHub Release
