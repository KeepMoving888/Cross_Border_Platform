# CB-Platform Kubernetes 部署方案(生产环境备用)

> **状态**:Phase 3 规划(备用方案,当前阶段使用 Docker Compose 微服务版)
> **适用条件**:团队 > 10 人 / 服务副本数 > 3 / 多环境隔离需求 / 金丝雀发布

## 一、为何当前不引入 K8s

| 评估维度 | 当前状态 | K8s 必要性 |
|---|---|---|
| 团队规模 | < 5 人 | ❌ Docker Compose 已足够 |
| 仓库结构 | 单仓库 | ❌ 无多团队协作需求 |
| 服务副本数 | 1-3 | ❌ 无需 HPA 自动扩缩容 |
| 多环境隔离 | 无 dev/staging/prod | ❌ |
| 运维成本 | K8s 控制面+etcd+Ingress+RBAC | ❌ 成本过高 |
| 故障隔离 | Docker Compose 容器隔离已满足 | ❌ |

**结论**:当前阶段 Docker Compose 微服务版已能支撑。K8s 作为生产环境备用方案,在以下条件触发时启用:
- 团队扩大到 10 人以上
- 单服务副本数需要 > 3(需 HPA 自动扩缩容)
- 需要多环境隔离(dev/staging/prod)
- 需要金丝雀发布或蓝绿部署
- 需要跨可用区高可用部署

## 二、目标架构(K8s 生产部署)

```
                    ┌──────────────────────┐
                    │   Ingress Controller │  ← Nginx Ingress / ALB
                    │   (TLS 终止 + 路由)  │
                    └──────────┬───────────┘
                               │
              ┌────────────────┼────────────────┐
              │                │                │
       ┌──────▼──────┐  ┌──────▼──────┐  ┌──────▼──────┐
       │ API Gateway │  │ AI Service  │  │ RAG Service │
       │ Deployment  │  │ Deployment  │  │ Deployment  │
       │ replicas:2  │  │ replicas:4  │  │ replicas:3  │
       │ HPA: QPS    │  │ HPA: CPU    │  │ HPA: QPS    │
       └──────┬──────┘  └──────┬──────┘  └──────┬──────┘
              │                │                │
              └────────────────┼────────────────┘
                               │
       ┌───────────────────────┼───────────────────────┐
       │                       │                       │
┌──────▼──────┐  ┌─────────────▼─────────────┐  ┌──────▼──────┐
│ MySQL       │  │ PostgreSQL + pgvector     │  │ Milvus      │
│ StatefulSet │  │ StatefulSet               │  │ StatefulSet │
│ 1主1从      │  │ 1主1从                    │  │ 集群模式    │
└─────────────┘  └───────────────────────────┘  └─────────────┘

┌─────────────┐  ┌─────────────┐  ┌─────────────┐
│ Redis       │  │ Prometheus  │  │ Grafana     │
│ Sentinel    │  │ Operator    │  │             │
└─────────────┘  └─────────────┘  └─────────────┘
```

## 三、资源规划

### 节点池规划

| 节点池 | 节点数 | 规格 | 污点/Toleration | 承载服务 |
|---|---|---|---|---|
| gateway-pool | 2 | 4C8G | 无 | API Gateway + Ingress |
| ai-pool | 2-4 | 8C16G | ai=true:NoSchedule | AI Service(autoscale 触发扩节点) |
| rag-pool | 2 | 8C16G | rag=true:NoSchedule | RAG Service |
| db-pool | 3 | 16C64G + SSD | db=true:NoSchedule | MySQL/PG/Redis StatefulSet |

### 资源 Request/Limit

| 服务 | CPU Request | CPU Limit | Mem Request | Mem Limit | 副本数 |
|---|---|---|---|---|---|
| API Gateway | 500m | 1000m | 512Mi | 1Gi | 2 |
| AI Service | 2000m | 4000m | 2Gi | 4Gi | 2-4(HPA) |
| RAG Service | 1000m | 2000m | 1Gi | 2Gi | 1-3(HPA) |
| MySQL | 1000m | 2000m | 1Gi | 2Gi | 1(主从) |
| PostgreSQL | 1000m | 2000m | 1Gi | 2Gi | 1(主从) |
| Redis | 500m | 1000m | 256Mi | 512Mi | 1(Sentinel) |
| Milvus | 2000m | 4000m | 4Gi | 8Gi | 1(集群) |

## 四、Helm Chart 结构(规划)

```
deployments/helm/
├── Chart.yaml                    # Chart 元数据
├── values.yaml                   # 默认配置
├── values-prod.yaml              # 生产环境覆盖
├── templates/
│   ├── _helpers.tpl              # 模板辅助函数
│   ├── gateway/
│   │   ├── deployment.yaml       # API Gateway Deployment
│   │   ├── service.yaml          # ClusterIP Service
│   │   ├── hpa.yaml              # HorizontalPodAutoscaler
│   │   ├── configmap.yaml        # 环境变量配置
│   │   └── networkpolicy.yaml    # 网络策略
│   ├── ai-service/
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   ├── hpa.yaml
│   │   └── configmap.yaml
│   ├── rag-service/
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   ├── hpa.yaml
│   │   └── configmap.yaml
│   ├── ingress.yaml              # Ingress 路由规则
│   ├── secret-registry.yaml      # 镜像仓库凭证
│   └── poddisruptionbudget.yaml  # PDB
└── charts/                       # 依赖子 Chart
    ├── mysql/                    # Bitnami MySQL
    ├── postgresql/               # Bitnami PostgreSQL(含 pgvector)
    ├── redis/                    # Bitnami Redis
    └── milvus/                   # Milvus Helm Chart
```

## 五、关键配置示例

### Deployment(gateway 示例)

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cb-gateway
  labels:
    app: cb-platform
    component: gateway
spec:
  replicas: 2
  selector:
    matchLabels:
      app: cb-platform
      component: gateway
  template:
    metadata:
      labels:
        app: cb-platform
        component: gateway
    spec:
      containers:
        - name: cb-gateway
          image: ghcr.io/keepmoving888/cross_border_platform:latest
          ports:
            - containerPort: 8080
          env:
            - name: APP_ROLE
              value: "gateway"
            - name: AI_SERVICE_URL
              value: "http://cb-ai-svc:8081"
            - name: RAG_SERVICE_URL
              value: "http://cb-rag-svc:8082"
            - name: MYSQL_HOST
              value: "cb-mysql"
            - name: REDIS_HOST
              value: "cb-redis"
          resources:
            requests:
              cpu: 500m
              memory: 512Mi
            limits:
              cpu: 1000m
              memory: 1Gi
          livenessProbe:
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 30
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 5
```

### HPA(AI Service 示例)

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: cb-ai-svc-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: cb-ai-svc
  minReplicas: 2
  maxReplicas: 4
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 60
      policies:
        - type: Percent
          value: 100
          periodSeconds: 60
    scaleDown:
      stabilizationWindowSeconds: 300
```

### Ingress(TLS + 路由)

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: cb-platform-ingress
  annotations:
    nginx.ingress.kubernetes.io/proxy-body-size: "50m"
    nginx.ingress.kubernetes.io/proxy-read-timeout: "300"
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
spec:
  tls:
    - hosts: [cb-platform.example.com]
      secretName: cb-platform-tls
  rules:
    - host: cb-platform.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: cb-gateway
                port:
                  number: 8080
```

## 六、渐进式迁移路径

### 阶段 1:K8s 集群搭建(1-2 周)
- 集群初始化(自建 kubeadm 或云厂商托管 K8s)
- 部署 Ingress Controller + cert-manager(Let's Encrypt)
- 部署 Prometheus Operator + Grafana
- 配置 CI/CD 流水线(镜像推送到 GHCR)

### 阶段 2:无状态服务迁移(1 周)
- 编写 Helm Chart(gateway/ai/rag 三个 Deployment)
- 配置 ConfigMap + Secret(环境变量 + 数据库凭证)
- 部署 HPA + PDB
- 验证 Ingress 路由和 TLS

### 阶段 3:有状态服务迁移(2-3 周)
- MySQL:StatefulSet + 主从复制(或使用云数据库 RDS)
- PostgreSQL:StatefulSet + pgvector 扩展(或使用云数据库 RDS)
- Redis:Sentinel 模式(或使用云数据库 Redis)
- Milvus:集群模式(向量规模 > 1M 时)

### 阶段 4:生产就绪(1 周)
- 配置 NetworkPolicy(服务间网络隔离)
- 配置 PodDisruptionBudget(滚动更新保障)
- 配置 PriorityClass(优先级调度)
- 配置备份策略(Velero 集群备份 + 数据库定期快照)
- 配置监控告警(Prometheus AlertManager)

## 七、CI/CD 流水线(K8s 模式)

```yaml
# .github/workflows/deploy-k8s.yml(规划,Phase 3 启用)
name: Deploy to K8s
on:
  push:
    tags: ['v*']
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Build and push image
        uses: docker/build-push-action@v5
        with:
          context: .
          file: deployments/Dockerfile
          push: true
          tags: ghcr.io/${{ github.repository }}:${{ github.ref_name }}
      - name: Deploy to K8s
        uses: azure/setup-kubectl@v4
        with:
          kubeconfig: ${{ secrets.KUBECONFIG }}
      - name: Helm upgrade
        run: |
          helm upgrade --install cb-platform deployments/helm \
            -f deployments/helm/values-prod.yaml \
            --set image.tag=${{ github.ref_name }}
```

## 八、监控与告警(K8s 模式)

| 监控维度 | 工具 | 关键指标 | 告警阈值 |
|---|---|---|---|
| 集群节点 | Prometheus + node-exporter | CPU/Mem/Disk 使用率 | > 80% |
| Pod 状态 | kube-state-metrics | Pod 重启次数 / Pending 数 | 重启 > 3 / Pending > 0 |
| API Gateway | Prometheus | P95 延迟 / 5xx 错误率 | P95 > 1s / 5xx > 1% |
| AI Service | Prometheus | 工作流 P95 延迟 / 失败率 | P95 > 30s / 失败 > 10% |
| RAG Service | Prometheus | 检索 P95 延迟 / 降级次数 | P95 > 2s / 降级 > 50 |
| MySQL | mysqld-exporter | 慢查询 / 连接数 | 慢查询 > 10 / 连接 > 80% |
| PostgreSQL | postgres-exporter | 连接数 / 缓存命中率 | 命中率 < 90% |

## 九、成本估算(参考)

| 资源 | 规格 | 月成本(估算) |
|---|---|---|
| K8s 控制面(托管) | 1 个 | $70 |
| gateway 节点(2台) | 4C8G | $120 |
| ai 节点(2-4台) | 8C16G | $240-480 |
| rag 节点(2台) | 8C16G | $240 |
| db 节点(3台) | 16C64G+SSD | $600 |
| 存储(SSD) | 500GB | $50 |
| 网络流量 | 1TB/月 | $50 |
| **合计** | | **$1370-1830/月** |

> 注:实际成本取决于云厂商、地域、预留实例折扣。相比 Docker Compose 单机部署(约 $300-500/月),K8s 成本约 3-4 倍,但获得高可用、自动扩缩容、滚动发布等能力。
