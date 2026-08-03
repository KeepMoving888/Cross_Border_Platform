# RAG Service 多副本水平扩展测试方案

> **状态**: 已完成单元/集成测试,待真实 K8s/Docker Swarm 集群验证
> **测试覆盖**: 并发安全 / 负载均衡 / 故障降级 / 熔断恢复

## 一、测试目标

验证 RAG Service 在多副本部署下的:
1. **并发安全性**: 高并发请求下熔断器状态机线程安全
2. **负载均衡**: 请求均匀分布到各副本
3. **故障容错**: 单副本故障时自动降级到本地 TF-IDF
4. **自动恢复**: 副本恢复后熔断器自动从 open→half-open→closed

## 二、测试环境

### 单元/集成测试(已通过)
- 环境: Go testing + httptest.Server
- 模拟: 多副本通过多 server 实例模拟
- 文件: [rag_client_scale_test.go](file:///C:/Users/Windows/AppData/Roaming/TRAE%20SOLO%20CN/ModularData/ai-agent/work-mode-projects/6a651e53f646f6881ff62012/cb-platform/internal/application/ai/rag_client_scale_test.go)

### Docker Compose 多副本测试(待执行)
```bash
# 启动 3 副本 RAG Service
docker compose -f deployments/docker-compose-microservices.yml \
  -f deployments/docker-compose-scale-test.yml up -d --scale cb-rag-svc=3

# 查看副本状态
docker compose -f deployments/docker-compose-microservices.yml ps cb-rag-svc

# 运行负载测试
go run ./cmd/loadtest \
  -url http://localhost:8082/api/v1/ai/rag/search \
  -concurrency 50 -duration 30s

# 查看各副本日志(验证请求分布)
docker compose -f deployments/docker-compose-microservices.yml logs cb-rag-svc
```

### K8s HPA 自动扩缩容测试(待 K8s 集群)
```bash
# 部署 Helm Chart
helm install cb-platform deployments/helm -f deployments/helm/values-prod.yaml

# 施加负载
go run ./cmd/loadtest -url http://<ingress>/api/v1/ai/rag/search \
  -concurrency 100 -duration 5m

# 观察 HPA 扩缩容
kubectl get hpa -w
```

## 三、测试用例与结果

### 测试 1: 高并发成功 (TestRemoteRAGClient_HighConcurrencySuccess)
- **场景**: 100 个并发请求同时访问 RAG Service
- **预期**: 全部成功,无 panic,无数据竞争
- **结果**: ✅ PASS - 100 并发,100 成功,0 失败

### 测试 2: 多副本负载均衡 (TestRemoteRAGClient_MultiReplicaLoadBalancing)
- **场景**: 3 个 RAG 副本,90 个请求轮询分发
- **预期**: 每副本处理约 30 请求,偏差 ≤ ±20%
- **结果**: ✅ PASS - 副本0:30, 副本1:30, 副本2:30, 偏差=0

### 测试 3: 副本故障降级 (TestRemoteRAGClient_ReplicaFailureTriggersFallback)
- **场景**: 1 个健康副本 + 1 个故障副本(503)
- **预期**: 故障副本请求降级到 TF-IDF,健康副本正常返回
- **结果**: ✅ PASS - 5 次请求全部降级,健康副本正常

### 测试 4: 熔断器恢复 (TestRemoteRAGClient_CircuitBreakerRecovery)
- **场景**: 副本先故障后恢复,观察熔断器状态转换
- **预期**: closed→open(故障)→half-open(等待)→closed(恢复)
- **结果**: ✅ PASS - 4 阶段状态转换全部正确

## 四、负载测试工具

[cmd/loadtest](file:///C:/Users/Windows/AppData/Roaming/TRAE%20SOLO%20CN/ModularData/ai-agent/work-mode-projects/6a651e53f646f6881ff62012/cb-platform/cmd/loadtest/main.go) 提供以下能力:

| 参数 | 说明 | 示例 |
|---|---|---|
| `-url` | RAG Service URL | `http://localhost:8082/api/v1/ai/rag/search` |
| `-concurrency` | 并发数 | `50` |
| `-duration` | 持续时间 | `30s`, `1m` |
| `-total` | 总请求数(duration=0 时生效) | `1000` |
| `-query` | 检索词 | `跨境物流时效` |

输出指标:
- 吞吐量 (req/s)
- 延迟分布 (Min/P50/P95/P99/Max)
- 状态码分布
- TraceID 分布(验证多副本负载均衡)

## 五、关键指标基线

| 指标 | 单副本基线 | 3 副本预期 | 说明 |
|---|---|---|---|
| P95 延迟 | < 500ms | < 500ms | 向量检索延迟不随副本数变化 |
| 吞吐量 | ~50 req/s | ~150 req/s | 线性扩展 |
| 错误率 | 0% | 0% | 无超时/熔断 |
| 故障恢复 | < 1s | < 1s | 熔断器 open_duration 后自动半开 |
