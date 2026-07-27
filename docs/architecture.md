# 架构设计文档

## 一、设计原则

1. **业务优先**:所有技术决策服务于业务落地,不做过度设计
2. **可观测**:每个关键操作都有日志、指标、追踪
3. **可扩展**:新增业务模块不影响现有模块,新增 AI 能力不影响现有工作流
4. **可独立部署**:单体应用 + 模块化设计,后续可平滑拆分为微服务
5. **AI 友好**:AI 工作流是一等公民,不是附属功能

## 二、分层架构

### 2.1 Interfaces 接口层

**职责**:协议适配、参数校验、响应封装

- HTTP Handler(Gin):接收 HTTP 请求,校验参数,调用应用层,返回统一响应
- 中间件链:TraceID → Recovery → Logger → Metrics → CORS → Auth

**规范**:
- Handler 不含业务逻辑,只做参数绑定、调用 Service、响应封装
- 统一响应格式:`{code, message, data, trace_id}`
- 统一错误码:1xxx 参数错误、2xxx 认证错误、5xxx 状态冲突、9xxx 系统错误

### 2.2 Application 应用层

**职责**:业务用例编排、跨领域协作

- AI 工作流引擎(`application/ai`):节点编排、执行器、LLM 调用
- 采购状态机(`application/purchase`):状态转换规则、合法性校验

**规范**:
- 应用服务编排领域对象,不直接操作数据库
- 事务边界在应用层控制

### 2.3 Domain 领域层

**职责**:实体、值对象、领域规则

- 实体:`Product`、`PurchaseOrder`、`Inventory`、`Bill`、`AIWorkflow` 等
- 值对象:`Pagination`、`TimeRange`、`CommonStatus`

**规范**:
- 实体包含业务字段与状态,不依赖任何框架
- GORM Tag 作为持久化元数据,与业务逻辑解耦

### 2.4 Infrastructure 基础设施层

**职责**:技术实现细节

- 数据库连接(`database`):MySQL、PostgreSQL、Redis 连接池管理
- 配置(`config`):Viper 多环境配置
- 日志(`logger`):Zap 结构化日志,支持控制台 + 文件 + 错误文件分离
- 错误(`errors`):统一错误码与 HTTP 状态映射
- 中间件(`middleware`):JWT 鉴权、CORS、限流、链路追踪、Prometheus 指标

## 三、采购流程状态机

### 3.1 状态定义

| 状态 | 名称 | 说明 |
|---|---|---|
| inquiry | 询价中 | 创建询价单,等待供应商报价 |
| quoting | 比价中 | 已收到报价,进行比价 |
| ordered | 已下单 | 选定报价,生成采购单 |
| shipped | 已发货 | 供应商已发货 |
| received | 已入库 | 货物入库验收 |
| qc | 质检中 | 质检流程 |
| reconciling | 对账中 | 财务对账 |
| settled | 已结算 | 完成结算,流程结束 |
| cancelled | 已取消 | 异常终止 |

### 3.2 状态转换规则

```
inquiry --quote--> quoting --select_quote--> ordered
inquiry --order--> ordered
ordered --ship--> shipped --receive--> received
received --qc--> qc --reconcile--> reconciling --settle--> settled
任何活跃状态 --cancel--> cancelled
cancelled --reopen--> inquiry
```

### 3.3 实现要点

- 使用 `looplab/fsm` 库,声明式定义状态转换规则
- 每次状态变更记录到 `purchase_status_logs` 表,支持审计
- 状态变更与业务操作在事务中执行(如入库时同时更新库存)
- 状态历史以 JSON 形式冗余在订单字段,便于快速查询

## 四、AI 工作流引擎

### 4.1 设计目标

- **可编排**:支持节点组合,定义复杂工作流
- **可观测**:每次执行记录到 `ai_workflow_runs` 表,包含输入/输出/Token/成本/耗时
- **可扩展**:新增节点类型只需实现 `Node` 接口
- **多 Provider**:运行时切换 LLM 提供商,不影响工作流定义

### 4.2 节点类型

| 类型 | 说明 | 实现 |
|---|---|---|
| input | 输入节点,接收外部参数 | 透传输入 |
| output | 输出节点,返回结果 | 字段映射 |
| llm | 大模型节点,调用 LLM | 支持 SystemPrompt + UserPrompt 模板 |
| rag | 检索增强节点,从知识库检索 | 关键词检索(生产环境用向量检索) |
| condition | 条件分支节点 | 简单 key=value 条件求值 |
| text2sql | 自然语言转 SQL | LLM + Schema 描述 |
| sql_execute | SQL 执行节点 | 安全检查 + 只允许 SELECT |
| tool | 工具节点 | 内置 getProduct/getSupplier 等 |

### 4.3 执行流程

```
1. 加载工作流定义(从 ai_workflows 表)
2. 解析 JSON 定义为 WorkflowDefinition
3. 构建节点实例(NodeFactory)
4. 拓扑遍历执行:
   - 从 input 节点开始
   - 执行当前节点,合并输出到 context
   - 根据边查找下一节点
   - 处理条件分支
5. 记录执行结果到 ai_workflow_runs
6. 返回 WorkflowResult
```

### 4.4 LLM Provider 抽象

```go
type LLMProvider interface {
    Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
    Name() string
}
```

支持的 Provider:
- GLM(智谱):国内首选,中文场景优秀
- Claude(Anthropic):代码与复杂推理强
- DeepSeek:性价比高
- Qwen(通义):阿里云生态
- OpenAI 兼容:通用协议
- Builtin:无 Key 时兜底(内置离线 Provider)

成本估算:基于各 Provider 公开定价,自动计算每次调用成本。

## 五、数据模型

### 5.1 核心表

| 表名 | 说明 | 关键索引 |
|---|---|---|
| users | 用户 | username(unique) |
| suppliers | 供应商 | code(unique), rating, coop_status |
| products | 选品 | sku(unique), stage, ai_score, category |
| inquiry_sheets | 询价单 | inquiry_no(unique), status |
| quotes | 报价 | inquiry_id, supplier_id |
| purchase_orders | 采购单 | order_no(unique), status, supplier_id |
| purchase_status_logs | 状态日志 | order_id |
| receive_records | 入库记录 | order_id, receive_no(unique) |
| warehouses | 仓库 | code(unique) |
| inventories | 库存 | sku+warehouse_id |
| inventory_movements | 库存流水 | sku, created_at |
| stock_alerts | 库存预警 | status |
| bills | 账单 | bill_no(unique), status, supplier_id |
| bill_items | 账单明细 | bill_id |
| profit_reports | 利润报表 | stat_date, sku, platform |
| ai_workflows | AI 工作流 | code(unique), scene |
| ai_workflow_runs | 工作流执行 | workflow_code, status, ref |
| knowledge_bases | 知识库 | code(unique) |
| knowledge_documents | 知识文档 | knowledge_base_id |
| prompt_templates | Prompt 模板 | code(unique), scene |

### 5.2 索引设计原则

- 主键:自增 ID(BigInt)
- 唯一约束:业务编号(order_no, sku, code 等)
- 查询索引:高频查询字段组合(stage+ai_score, status+created_at)
- 软删除:全部表支持 deleted_at 软删除

## 六、可观测性

### 6.1 日志

- 结构化 JSON 格式(Zap)
- 三路输出:控制台 + app.log + error.log
- 字段:timestamp、level、caller、message、trace_id、业务字段
- 请求日志:method、path、status、latency_ms、client_ip、user_id

### 6.2 指标(Prometheus)

- `http_requests_total`:请求总数(method, path, status)
- `http_request_duration_seconds`:请求耗时直方图
- `http_requests_in_flight`:当前处理中请求数
- `http_response_size_bytes`:响应大小

### 6.3 链路追踪

- 每个请求生成唯一 `trace_id`
- 通过 `X-Trace-Id` Header 透传
- 日志与响应都包含 trace_id,便于排查

### 6.4 健康检查

- `/health`:应用 + 数据库连接状态
- `/ping`:轻量级存活检查
- Docker healthcheck:30 秒一次

## 七、安全设计

- 密码:bcrypt 哈希存储
- 鉴权:JWT + 过期机制
- 权限:RBAC 角色模型(admin / manager / staff)
- SQL 注入:GORM 参数化查询,Text2SQL 节点强制 SELECT only
- 跨域:CORS 中间件白名单
- 限流:令牌桶限流(基于 IP)
- 敏感信息:RefreshToken、AccessToken 不返回前端
