# API 接口文档

## 通用说明

### 基础信息

- Base URL: `http://localhost:8080/api/v1`
- 请求格式: `application/json`
- 鉴权方式: `Authorization: Bearer <token>`

### 统一响应格式

```json
{
  "code": 0,
  "message": "success",
  "data": {},
  "trace_id": "uuid-v4"
}
```

| 字段 | 说明 |
|---|---|
| code | 0=成功,非 0=业务错误码 |
| message | 提示信息 |
| data | 业务数据 |
| trace_id | 链路追踪 ID |

### 错误码

| 范围 | 说明 |
|---|---|
| 0 | 成功 |
| 1001-1999 | 参数错误 |
| 2001-2999 | 认证错误 |
| 3001-3999 | 权限错误 |
| 4001-4999 | 资源不存在 |
| 5001-5999 | 状态冲突 |
| 9001-9999 | 系统错误 |

### 分页参数

| 参数 | 默认 | 说明 |
|---|---|---|
| page | 1 | 页码 |
| page_size | 20 | 每页条数(最大 200) |

分页响应:
```json
{
  "code": 0,
  "data": {
    "list": [],
    "total": 100,
    "page": 1,
    "page_size": 20
  }
}
```

---

## 认证模块

### 登录

`POST /auth/login`

请求:
```json
{
  "username": "admin",
  "password": "admin123"
}
```

响应:
```json
{
  "code": 0,
  "data": {
    "token": "eyJhbGc...",
    "user_info": {
      "id": 1,
      "username": "admin",
      "role": "admin"
    }
  }
}
```

### 注册

`POST /auth/register`

### 刷新 Token

`POST /auth/refresh`(需登录)

---

## 选品管理

### 选品列表

`GET /products`

查询参数:
- keyword: 关键词
- category: 类目
- stage: sourcing / testing / approved / rejected / archived
- platform: 平台
- min_score: 最低 AI 评分
- page / page_size

### 创建选品

`POST /products`

```json
{
  "sku": "SKU-001",
  "name": "便携蓝牙音箱",
  "category": "电子产品",
  "platform": "amazon",
  "target_market": "US",
  "list_price": 39.99,
  "est_cost_price": 12.50,
  "currency": "USD"
}
```

### AI 选品分析

`POST /products/:id/analyze`

调用 `wf_product_analysis` 工作流,返回评分与建议,并更新商品的 ai_score 字段。

### 变更选品阶段

`PUT /products/:id/stage`

```json
{
  "stage": "approved",
  "remark": "通过测试,可进入采购"
}
```

---

## 采购管理

### 询价单

- `GET /purchases/inquiries` 列表
- `POST /purchases/inquiries` 创建
- `GET /purchases/inquiries/:id` 详情(含报价)
- `PUT /purchases/inquiries/:id` 更新
- `DELETE /purchases/inquiries/:id` 删除(仅 draft)
- `POST /purchases/inquiries/:id/close` 关闭

### 报价

- `GET /purchases/inquiries/:id/quotes` 列表
- `POST /purchases/quotes` 提交报价
- `PUT /purchases/quotes/:id/select` 选定报价

### 采购单

- `GET /purchases/orders` 列表
- `POST /purchases/orders` 创建
- `GET /purchases/orders/:id` 详情(含状态日志与入库记录)
- `PUT /purchases/orders/:id` 更新(仅 inquiry/quoting 状态)
- `POST /purchases/orders/:id/transition` 状态机驱动状态变更
- `POST /purchases/orders/:id/receive` 入库
- `GET /purchases/orders/:id/logs` 状态变更日志

#### 状态变更

`POST /purchases/orders/:id/transition`

```json
{
  "event": "ship",
  "logistics_no": "SF1234567890",
  "logistics_company": "顺丰",
  "remark": "供应商已发货"
}
```

合法事件:
- `quote`: 询价中 → 比价中
- `select_quote`: 比价中 → 已下单
- `order`: 询价/比价 → 已下单
- `ship`: 已下单 → 已发货
- `receive`: 已下单/已发货/跟单 → 已入库
- `qc`: 已入库 → 质检中
- `reconcile`: 质检/已入库 → 对账中
- `settle`: 对账中 → 已结算
- `cancel`: 任意活跃状态 → 已取消
- `reopen`: 已取消 → 询价中

---

## 库存管理

### 库存

- `GET /inventory` 库存列表
- `GET /inventory/:id` 详情
- `PUT /inventory/:id` 更新(安全库存等)
- `POST /inventory/adjust` 库存调整(原子操作 + 流水)

#### 库存调整

`POST /inventory/adjust`

```json
{
  "warehouse_id": 1,
  "sku": "SKU-001",
  "quantity": 100,
  "type": "inbound",
  "remark": "采购入库"
}
```

type 取值:inbound / outbound / adjust / return

### 流水与预警

- `GET /inventory/movements` 流水列表
- `GET /inventory/alerts` 预警列表
- `POST /inventory/alerts/:id/resolve` 处理预警

### 仓库

- `GET /inventory/warehouses` 仓库列表
- `POST /inventory/warehouses` 创建
- `PUT /inventory/warehouses/:id` 更新

---

## 对账与利润

### 账单

- `GET /finance/bills` 列表
- `POST /finance/bills` 创建
- `GET /finance/bills/:id` 详情(含明细)
- `PUT /finance/bills/:id` 更新
- `POST /finance/bills/:id/match` 对账(自动比对差异)
- `POST /finance/bills/:id/pay` 付款
- `GET /finance/bills/:id/items` 明细列表

### 利润报表

- `GET /finance/profit/summary` 汇总
- `GET /finance/profit/by-sku` 按 SKU
- `GET /finance/profit/by-platform` 按平台
- `GET /finance/profit/trend` 趋势(默认 30 天)

---

## AI 工作流

### 工作流管理

- `GET /ai/workflows` 列表
- `POST /ai/workflows` 创建
- `GET /ai/workflows/:id` 详情(含最近执行)
- `PUT /ai/workflows/:id` 更新
- `POST /ai/workflows/:id/run` 执行工作流

#### 执行工作流

`POST /ai/workflows/:id/run`

```json
{
  "input": {
    "category": "电子产品",
    "market": "US",
    "product": "蓝牙音箱"
  }
}
```

### 执行历史

- `GET /ai/runs` 列表
- `GET /ai/runs/:id` 详情

### Prompt 模板

- `GET /ai/prompts` 列表
- `POST /ai/prompts` 创建
- `PUT /ai/prompts/:id` 更新

### 知识库

- `GET /ai/knowledge-bases` 列表
- `POST /ai/knowledge-bases` 创建
- `POST /ai/knowledge-bases/:id/documents` 上传文档
- `GET /ai/knowledge-bases/:id/documents` 文档列表

### 业务场景直调

- `POST /ai/analyze/product` AI 选品分析
- `POST /ai/generate/listing` Listing 生成
- `POST /ai/reply/customer` 客服回复

---

## 数据看板

- `GET /dashboard/overview` 总览(首页核心指标)
- `GET /dashboard/product/stats` 选品统计
- `GET /dashboard/purchase/stats` 采购统计
- `GET /dashboard/inventory/stats` 库存统计
- `GET /dashboard/profit/stats` 利润统计
- `GET /dashboard/ai/stats` AI 使用统计

---

## 平台对接

- `GET /platform/accounts` 平台账号列表
- `POST /platform/accounts` 添加账号
- `PUT /platform/accounts/:id` 更新
- `POST /platform/accounts/:id/sync` 触发数据同步
- `GET /platform/accounts/:id/products` 平台商品(预留)
- `GET /platform/accounts/:id/orders` 平台订单(预留)

---

## 健康检查

- `GET /health` 应用与数据库健康状态
- `GET /ping` 存活检查
- `GET /metrics` Prometheus 指标
