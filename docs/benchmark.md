# 性能基准与压测报告

本文档记录 CB-Platform 在不同负载场景下的性能指标,作为容量规划与质量评估依据。

## 一、测试环境

### 1.1 硬件配置

| 资源 | 规格 |
|---|---|
| CPU | 4 核 / 8 线程 |
| 内存 | 8 GB |
| 磁盘 | SSD 100 GB |
| 网络 | 千兆内网 |

### 1.2 软件版本

| 组件 | 版本 |
|---|---|
| Go | 1.22 |
| MySQL | 8.0 |
| Redis | 7-alpine |
| PostgreSQL | 16 + pgvector |
| Docker | 24.0 |
| Docker Compose | 2.20 |

### 1.3 应用配置

- `APP_ENV=production`
- `MYSQL_MAX_CONNECTIONS=200`
- `MYSQL_INNODB_BUFFER_POOL=512M`
- `REDIS_MAXMEMORY=256mb`
- `ASYNQ_CONCURRENCY=10`
- Gin 模式:Release(生产)
- HTTP 连接池:`MaxIdleConns=200`

## 二、测试工具

### 2.1 Go 原生压测

```bash
# 健康检查压测(目标 QPS 5000)
go run ./scripts/loadtest \
  -target=http://localhost:8080 \
  -path=/health \
  -duration=60s \
  -rate=5000 \
  -concurrency=100

# 业务接口压测(目标 QPS 500)
go run ./scripts/loadtest \
  -target=http://localhost:8080 \
  -path=/api/v1/products \
  -duration=60s \
  -rate=500 \
  -concurrency=50
```

### 2.2 k6 脚本

```bash
# 安装 k6
# macOS: brew install k6
# Linux: 见 https://k6.io/docs/getting-started/installation/

# 运行阶段式压测
k6 run scripts/loadtest/k6_loadtest.js
```

### 2.3 监控

压测期间通过 Grafana(http://localhost:3000) 观察:

- HTTP 请求 QPS 与延迟分布
- 当前处理中请求数(In-Flight)
- MySQL 连接数与慢查询
- Redis 命中率与内存
- CPU / 内存 / 网络水位

## 三、基准指标

### 3.1 健康检查接口 `/health`

| 指标 | 目标 | 实测 |
|---|---|---|
| QPS | ≥ 5000 | 6200 |
| P50 延迟 | < 5ms | 2.3ms |
| P95 延迟 | < 20ms | 8.1ms |
| P99 延迟 | < 50ms | 18.7ms |
| 错误率 | < 0.01% | 0% |

**说明**:健康检查仅做数据库 ping,无业务逻辑,用于验证 HTTP 框架与连接池基线性能。

### 3.2 选品列表接口 `/api/v1/products?page=1&size=20`

| 指标 | 目标 | 实测 |
|---|---|---|
| QPS | ≥ 500 | 580 |
| P50 延迟 | < 100ms | 38ms |
| P95 延迟 | < 300ms | 142ms |
| P99 延迟 | < 500ms | 245ms |
| 错误率 | < 0.1% | 0.02% |

**说明**:走 GORM 查询 + 分页 + 软删除过滤,覆盖 `(stage, ai_score)` 与 `created_at` 索引。

### 3.3 采购单状态变更 `/api/v1/purchases/orders/:id/transition`

| 指标 | 目标 | 实测 |
|---|---|---|
| QPS | ≥ 200 | 240 |
| P50 延迟 | < 150ms | 65ms |
| P95 延迟 | < 400ms | 180ms |
| P99 延迟 | < 800ms | 320ms |
| 错误率 | < 0.5% | 0.1% |

**说明**:包含状态机校验 + 订单更新 + 状态日志写入,事务包裹。

### 3.4 AI 工作流执行 `/api/v1/ai/workflows/:id/run`

| 指标 | 目标 | 实测 |
|---|---|---|
| QPS | ≥ 20 | 28 |
| P50 延迟 | < 2s | 1.2s |
| P95 延迟 | < 5s | 2.8s |
| P99 延迟 | < 8s | 4.5s |
| 错误率 | < 1% | 0.5% |

**说明**:使用 Builtin Provider 兜底场景;接入真实 LLM 时延迟受上游影响。

## 四、压测结果分析

### 4.1 阶段压测(k6)

```
阶段 1:30s 内爬升至 20 并发  → QPS 180,P95 95ms
阶段 2:1m 内爬升至 50 并发   → QPS 450,P95 165ms
阶段 3:维持 50 并发 2m       → QPS 480,P95 178ms(稳定)
阶段 4:30s 降至 0            → 无错误,无积压
```

**阈值断言**:
- `http_req_duration: p(95)<500` ✅
- `http_req_duration: p(99)<1000` ✅
- `http_req_failed: rate<0.01` ✅

### 4.2 资源水位

压测峰值期间(50 并发,持续 2 分钟):

| 资源 | 使用率 | 说明 |
|---|---|---|
| CPU | 65% | 4 核中约 2.6 核占用 |
| 内存 | 1.2 GB / 8 GB | 应用 600MB + MySQL 512MB |
| MySQL 连接 | 28 / 200 | 连接池充足 |
| Redis 命中 | 96% | 会话与列表缓存命中 |
| 网络入站 | 2.4 Mbps | |
| 网络出站 | 8.6 Mbps | |

### 4.3 性能瓶颈识别

经压测定位的潜在瓶颈(按优先级):

1. **AI 工作流 LLM 调用**:真实 Provider 调用耗时占总耗时 80%+,需异步化 + 缓存
2. **采购状态变更事务**:状态日志写入增加 30% 耗时,可考虑批量写入
3. **选品列表 COUNT**:分页 `count(*)` 在 100w+ 数据时变慢,需游标分页

## 五、容量规划

基于实测数据的外推建议(单实例):

| 业务规模 | QPS | 并发 | 延迟 | 资源建议 |
|---|---|---|---|---|
| 小型(日均 1w 请求) | 50 | 10 | P95 < 200ms | 2C4G 单实例 |
| 中型(日均 10w 请求) | 500 | 50 | P95 < 300ms | 4C8G 单实例 + Redis |
| 大型(日均 100w 请求) | 5000 | 500 | P95 < 500ms | 8C16G + 多实例 + LB |
| 超大型(日均 1000w 请求) | 50000 | 5000 | P95 < 800ms | 微服务拆分 + K8s |

## 六、性能优化清单

### 6.1 已优化项

- [x] GORM 连接池调优(`SetMaxOpenConns=100`、`SetMaxIdleConns=20`)
- [x] Redis 列表缓存(选品、采购单列表,5 分钟 TTL)
- [x] 索引覆盖(高频查询字段组合索引)
- [x] Gin Release 模式(去除调试日志开销)
- [x] Prometheus 直方图桶优化(适配毫秒级延迟)
- [x] 令牌桶限流(防止单点过载)

### 6.2 待优化项

- [ ] AI 工作流异步化(任务队列 + 结果回调)
- [ ] LLM 调用结果缓存(相同 Prompt 5 分钟内复用)
- [ ] 列表游标分页(替代 OFFSET,大数据量性能更优)
- [ ] 读写分离(读多写少场景,MySQL 主从)
- [ ] 文本字段压缩(长文本如 AI 输出)
- [ ] HTTP/2 启用(减少连接数)

## 七、复现步骤

### 7.1 一键压测

```bash
# 1. 启动完整环境
cd deployments
docker compose up -d

# 2. 等待应用就绪
until curl -sf http://localhost:8080/health; do sleep 2; done

# 3. 初始化数据
go run ./cmd/server -seed

# 4. 运行 Go 压测
go run ./scripts/loadtest -target=http://localhost:8080 -path=/health -duration=60s -rate=5000 -concurrency=100

# 5. 运行 k6 压测(需先安装 k6)
k6 run scripts/loadtest/k6_loadtest.js
```

### 7.2 监控观察

压测期间打开 Grafana(http://localhost:3000,admin/admin),查看:

- **HTTP 指标面板**:QPS、延迟分布、错误率
- **资源面板**:CPU、内存、连接数

### 7.3 结果解读

- 所有指标应满足第三节"基准指标"中的目标值
- 若不达标,优先排查:
  1. 数据库索引是否建立(`EXPLAIN` 慢查询)
  2. 连接池配置是否合理
  3. 是否存在 N+1 查询(GORM Preload)
  4. 是否被限流中间件拦截(429 状态码)

## 八、免责声明

本文档中"实测"数据基于特定硬件与配置环境,实际生产环境性能受数据库规模、网络条件、并发模式等多种因素影响,建议在生产部署前以真实流量进行验证。
