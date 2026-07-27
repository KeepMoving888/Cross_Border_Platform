# 贡献指南

感谢您对 CB-Platform 项目的关注!本文档描述了参与项目开发需要遵循的规范与流程。

## 开发环境准备

### 必要工具

- Go 1.22+
- Docker 20.10+ 与 Docker Compose 2.0+
- Make(可选,用于执行常用命令)
- golangci-lint v1.60+(推荐,用于代码检查)

### 本地启动

```bash
# 克隆代码
git clone https://github.com/cb-platform/cb-platform.git
cd cb-platform

# 安装 Go 依赖
go mod download

# 启动基础设施(MySQL/Redis/PostgreSQL/Prometheus/Grafana)
make docker-up

# 复制配置文件
cp .env.example .env

# 运行应用(自动迁移数据库 + 初始化种子数据)
make run
```

应用启动后访问:
- API: http://localhost:8080
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000 (admin/admin)

## 代码规范

### Go 代码风格

1. **格式化**:所有代码必须通过 `gofmt -s` 格式化
2. **导入顺序**:标准库 / 第三方库 / 项目内包(用空行分隔)
3. **命名**:遵循 [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
   - 包名:小写单数,简短
   - 导出标识符:驼峰命名,首字母大写
   - 未导出标识符:驼峰命名,首字母小写
4. **错误处理**:错误必须显式处理,避免 `_` 忽略;错误先返回,避免嵌套
5. **注释**:导出函数/类型/常量必须有注释,以名称开头

### 项目结构

遵循 DDD 分层架构:

```
internal/
├── domain/          # 领域层:实体、值对象、领域服务
│   ├── models/      # 数据模型
│   └── platform/    # 平台对接抽象
├── application/     # 应用层:用例编排、状态机、AI 工作流
│   ├── ai/          # AI 工作流引擎
│   └── purchase/    # 采购状态机
├── infrastructure/  # 基础设施层
│   └── pkg/
│       ├── config/  # 配置管理
│       ├── database/# 数据库连接
│       ├── logger/  # 日志
│       ├── errors/  # 错误处理
│       ├── response/# 统一响应
│       └── middleware/
└── interfaces/      # 接口层
    └── http/
        ├── server.go
        └── handler/
```

**依赖方向**:interfaces → application → domain ← infrastructure

### 数据库

1. **表结构变更**:在 `migrations/` 目录新增 SQL 文件,命名 `XXX_description.sql`
2. **GORM 模型**:在 `internal/domain/models/` 中定义,使用 `BaseModel` 嵌入
3. **索引**:为常用查询字段建立索引,避免全表扫描
4. **字段类型**:金额使用 `decimal.Decimal`,时间使用 `*time.Time`(可空)

### API 设计

1. **RESTful**:遵循 REST 风格,资源用名词,动作用 HTTP 方法
2. **版本**:统一 `/api/v1/` 前缀
3. **响应格式**:统一使用 `response.OK` / `response.Fail`
4. **错误码**:遵循预定义错误码段(1xxx 参数 / 2xxx 认证 / 4xxx 资源 / 9xxx 系统)
5. **分页**:使用 `page` + `size` 参数,返回 `PageResult`

## 测试要求

### 单元测试

- 所有新增功能必须配套单元测试
- 测试文件与被测文件同目录,命名 `xxx_test.go`
- 测试覆盖率目标 ≥ 70%(核心包 ≥ 85%)
- 使用 table-driven tests 风格

```bash
# 运行所有测试
make test

# 生成覆盖率报告
make coverage
```

### 测试规范

1. **命名**:`TestFuncName_Scenario` 格式,如 `TestStateMachine_LegalTransition`
2. **断言**:使用标准库 `testing`,避免引入额外断言库
3. **隔离**:测试之间不应有依赖,使用临时目录或独立数据
4. **清理**:`defer` 释放资源,避免泄漏

## 提交规范

### Commit Message

遵循 [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <subject>

<body>

<footer>
```

**type** 取值:
- `feat`:新功能
- `fix`:bug 修复
- `docs`:文档变更
- `style`:代码格式(不影响功能)
- `refactor`:重构
- `perf`:性能优化
- `test`:测试相关
- `chore`:构建/工具变更

**示例**:
```
feat(purchase): add purchase order receive endpoint

- 新增 POST /api/v1/purchases/orders/:id/receive 接口
- 入库时同步更新库存与流水
- 关联状态机 receive 事件,确保状态合法

Closes #123
```

### PR 流程

1. **Fork & Clone**:Fork 项目到个人仓库
2. **创建分支**:`feat/xxx` / `fix/xxx` / `docs/xxx`
3. **开发**:
   - 遵循代码规范
   - 编写单元测试
   - 更新相关文档
4. **自测**:
   ```bash
   make lint       # 代码检查
   make test       # 运行测试
   make build      # 编译验证
   ```
5. **提交 PR**:
   - 标题遵循 commit message 规范
   - 描述清晰:背景、改动、影响、测试方式
   - 关联相关 Issue

### PR 审查要点

- 代码是否符合规范
- 是否有单元测试覆盖
- 是否影响现有功能
- 是否引入安全风险(如硬编码密钥、SQL 注入)
- 性能影响如何(数据库查询、内存分配)

## 安全规范

1. **密钥管理**:
   - 严禁硬编码密钥、Token、密码
   - 使用环境变量或配置文件(已在 `.gitignore` 中排除)
   - 提交前检查 `git diff` 是否包含敏感信息

2. **输入校验**:
   - 所有外部输入必须校验(Gin binding + 业务校验)
   - SQL 查询使用参数化,GORM 已默认防注入
   - 文件上传校验类型、大小

3. **权限控制**:
   - 受保护接口必须使用 `middleware.Auth()`
   - 敏感操作记录审计日志
   - 越权访问返回 403

4. **依赖安全**:
   - 定期运行 `govulncheck`(CI 已集成)
   - 及时更新存在漏洞的依赖

## 发布流程

### 版本号

遵循 [Semantic Versioning](https://semver.org/):
- `MAJOR.MINOR.PATCH`
- 不兼容变更:`MAJOR` + 1
- 新增功能(向下兼容):`MINOR` + 1
- Bug 修复(向下兼容):`PATCH` + 1

### 发布步骤

1. 更新 `CHANGELOG.md`
2. 创建 tag:`git tag -a v1.0.0 -m "Release v1.0.0"`
3. 推送 tag:`git push origin v1.0.0`
4. CI 自动构建并创建 GitHub Release

## 社区行为准则

- 尊重所有参与者,不论技术水平
- 保持开放、协作的态度
- 接受建设性批评
- 关注项目与社区的整体利益

## 联系方式

- Bug 反馈:[GitHub Issues](https://github.com/cb-platform/cb-platform/issues)
- 功能建议:[GitHub Discussions](https://github.com/cb-platform/cb-platform/discussions)
- 安全漏洞:请勿公开 Issue,邮件至 security@cb-platform.example
