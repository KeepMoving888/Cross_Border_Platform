# 部署指南

## 一、Docker Compose 部署(推荐)

### 1.1 完整部署

```bash
cd deployments
docker compose up -d
```

启动后包含以下服务:

| 服务 | 端口 | 容器名 |
|---|---|---|
| CB-Platform | 8080 | cb-platform-app |
| MySQL | 3306 | cb-platform-mysql |
| Redis | 6379 | cb-platform-redis |
| PostgreSQL | 5432 | cb-platform-postgres |
| Prometheus | 9090 | cb-platform-prometheus |
| Grafana | 3000 | cb-platform-grafana |

### 1.2 验证

```bash
# 健康检查
curl http://localhost:8080/health

# 登录获取 Token
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'

# 使用 Token 调用接口
curl http://localhost:8080/api/v1/dashboard/overview \
  -H "Authorization: Bearer <token>"
```

### 1.3 配置 LLM

编辑 `docker-compose.yml` 中 `cb-platform` 服务的环境变量:

```yaml
environment:
  - LLM_PROVIDER=glm
  - LLM_API_KEY=your_actual_api_key
  - LLM_BASE_URL=https://open.bigmodel.cn/api/paas/v4
  - LLM_MODEL=glm-4-plus
```

或通过 `.env` 文件传入:

```bash
# deployments 目录下创建 .env
LLM_API_KEY=your_actual_api_key

docker compose up -d
```

### 1.4 查看日志

```bash
# 应用日志
docker compose logs -f cb-platform

# 进入容器查看日志文件
docker exec -it cb-platform-app sh
cat /app/logs/app.log
cat /app/logs/error.log
```

### 1.5 停止与清理

```bash
# 停止
docker compose down

# 停止并删除数据卷(谨慎!)
docker compose down -v
```

## 二、本地开发部署

### 2.1 启动基础设施

```bash
make docker-up
```

仅启动 MySQL/Redis/PG,应用本地运行。

### 2.2 配置

```bash
cp .env.example .env
```

修改 `.env`:

```bash
APP_ENV=development
APP_PORT=8080
MYSQL_HOST=127.0.0.1
MYSQL_PORT=3306
MYSQL_USER=cb
MYSQL_PASSWORD=cb_pass_2026
MYSQL_DB=cb_platform
REDIS_HOST=127.0.0.1
REDIS_PORT=6379
LLM_PROVIDER=glm
LLM_API_KEY=your_key
```

### 2.3 运行

```bash
# 首次运行:迁移 + 种子数据
go run ./cmd/server -seed

# 后续运行
make run
```

## 三、生产环境部署建议

### 3.1 资源规划

| 服务 | CPU | 内存 | 磁盘 |
|---|---|---|---|
| CB-Platform | 1 核 | 1G | 10G(日志) |
| MySQL | 2 核 | 2G | 50G+ |
| Redis | 0.5 核 | 256M | 1G |
| PostgreSQL | 1 核 | 1G | 10G |

### 3.2 安全加固

1. **修改默认密码**:
   - MySQL root 密码
   - admin 用户密码(登录后立即修改)
   - JWT Secret
   - Grafana admin 密码

2. **网络隔离**:
   - 数据库不对外暴露端口,仅允许应用容器访问
   - Prometheus/Grafana 加反向代理 + Basic Auth

3. **HTTPS**:
   - 生产环境必须配置 TLS
   - 使用 Nginx/Caddy 反向代理

4. **密钥管理**:
   - LLM API Key 通过环境变量注入,不写入代码
   - JWT Secret 使用 32+ 字符随机串

### 3.3 数据备份

```bash
# MySQL 备份
docker exec cb-platform-mysql mysqldump -u cb -pcb_pass_2026 cb_platform > backup_$(date +%Y%m%d).sql

# 定时备份(crontab)
0 2 * * * docker exec cb-platform-mysql mysqldump -u cb -pcb_pass_2026 cb_platform > /backup/backup_$(date +\%Y\%m\%d).sql
```

### 3.4 监控告警

1. **Grafana 配置告警**:
   - HTTP 5xx 错误率 > 1% 告警
   - 接口平均响应时间 > 500ms 告警
   - 数据库连接失败告警

2. **关键指标**:
   - `http_requests_total`:按 status 维度查看错误率
   - `http_request_duration_seconds`:P99 响应时间
   - `http_requests_in_flight`:并发数

## 四、平台 API 对接说明

当前阶段平台 API(亚马逊 SP-API / Temu / TikTok Shop)接口已预留,通过 `PlatformHandler` 提供统一入口。

### 4.1 对接步骤(后续)

1. **OAuth 授权**:
   - 亚马逊:通过 SP-API 完成 LWA(Launchpad for Authorization)授权
   - Temu:对接卖家后台开放平台
   - TikTok Shop:对接 Shop API

2. **填充 Adapter**:
   ```go
   // internal/infrastructure/external/amazon/adapter.go
   type AmazonAdapter struct {
       refreshToken string
       // ...
   }
   func (a *AmazonAdapter) SyncProducts() error { ... }
   func (a *AmazonAdapter) SyncOrders() error { ... }
   ```

3. **接入 SyncAccount**:
   ```go
   func (h *PlatformHandler) SyncAccount(c *gin.Context) {
       adapter := external.NewAdapter(acc.Platform, acc.RefreshToken)
       go adapter.SyncProducts()  // 异步同步
   }
   ```

4. **数据同步策略**:
   - 增量同步:基于 last_updated 时间戳
   - 全量同步:每日凌晨定时任务
   - 失败重试:Asynq 任务队列,指数退避

### 4.2 当前行为

未接入时调用 `SyncAccount` 返回:
```json
{
  "code": 0,
  "message": "数据同步任务已提交(平台 API 对接预留)",
  "data": {
    "account_id": 1,
    "platform": "amazon",
    "status": "queued",
    "message": "平台 API 已预留对接接口,后续接入 OAuth 与 SP-API 即可启用"
  }
}
```
