# 安全策略

## 报告安全漏洞

CB-Platform 重视安全问题。如果您发现安全漏洞,请按以下流程报告, **请勿通过公开 Issue 提交**:

1. 发送邮件至 `security@cb-platform.example`
2. 邮件主题:`[SECURITY] 简要描述`
3. 邮件内容请包含:
   - 漏洞影响范围与严重程度
   - 复现步骤(详细命令或代码片段)
   - 影响版本
   - 建议的修复方案(可选)

我们将在 **3 个工作日内**响应,并在确认漏洞后:

- 评估影响范围与严重等级(CVSS 评分)
- 在 `SECURITY.md` 与 `CHANGELOG.md` 中记录
- 通过补丁版本发布修复

## 支持版本

| 版本 | 状态 | 安全更新 |
|---|---|---|
| 1.0.x | ✅ 当前 | 接收安全补丁 |
| < 1.0 | ❌ 不支持 | 请升级到最新版 |

## 已知安全实践

### 密钥与凭证

- **禁止硬编码**:所有密钥、Token、密码通过环境变量或 `.env` 文件注入
- **`.env` 已在 `.gitignore` 中排除**,仅 `.env.example` 入库(不含真实凭证)
- **生产环境**建议使用密钥管理服务(如 AWS Secrets Manager、HashiCorp Vault)

### 鉴权与权限

- 密码使用 `bcrypt` 哈希存储(`golang.org/x/crypto/bcrypt`,cost=10)
- JWT Token 默认 24 小时过期,可通过 `JWT_EXPIRE_HOURS` 配置
- 受保护接口必须经过 `middleware.Auth()` 中间件校验
- 越权访问返回 `403 Forbidden`

### 输入校验

- 所有 HTTP 请求参数通过 Gin `binding` 标签校验
- SQL 查询使用 GORM 参数化,防止 SQL 注入
- AI 工作流的 Text2SQL 节点强制 `SELECT only`,拒绝写操作

### 限流与防滥用

- 内置令牌桶限流(基于 IP),可通过 `middleware.RateLimiter(rps)` 配置
- AI 工作流调用记录 Token 消耗,便于成本控制

### 依赖安全

- CI 集成 `govulncheck` 与 `gosec`,自动扫描已知漏洞
- 依赖更新通过 Dependabot / Renovate 自动提报 PR

## 部署安全加固

生产环境部署时建议:

1. **网络隔离**:应用与数据库不同子网,仅开放必要端口
2. **TLS 加密**:前置 Nginx / Caddy 启用 HTTPS
3. **数据库**:MySQL/PG 启用 SSL 连接,Redis 启用密码与 ACL
4. **容器**:以非 root 用户运行(Dockerfile 已配置)
5. **审计**:开启 MySQL 慢查询日志、应用访问日志
6. **备份**:每日全量备份 + binlog 增量备份

详细部署加固见 [docs/deployment.md](docs/deployment.md)。

## 安全相关配置项

```bash
# .env(示例,生产环境请用密钥管理服务)
JWT_SECRET=<strong_random_secret_at_least_32_chars>
MYSQL_PASSWORD=<strong_password>
REDIS_PASSWORD=<strong_password>
PG_PASSWORD=<strong_password>
LLM_API_KEY=<your_llm_provider_key>
```

## 致谢

感谢以下安全研究者对 CB-Platform 的贡献(按报告时间排序):

- _(暂无)_

如您的报告被确认,可在征得同意后在此处署名。
