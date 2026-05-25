# 云服务商部署指南

本指南覆盖主流云平台上部署 CopCon 的推荐架构,重点在于利用托管服务减少运维负担。

## AWS

### 推荐架构

```
┌────────────┐     ┌──────────────┐     ┌────────────────┐
│ CloudFront │────▶│   ALB        │────▶│  ECS Fargate   │
│ (CDN/TLS)  │     │ (Load Bal.)  │     │  copcon-server │
└────────────┘     └──────────────┘     └───────┬────────┘
                                                 │
                          ┌──────────────────────┼──────────────┐
                          ▼                      ▼              ▼
                   ┌────────────┐        ┌────────────┐ ┌──────────┐
                   │ RDS        │        │ ElastiCache│ │ Bedrock  │
                   │ PostgreSQL │        │ (Redis)    │ │ (LLM)    │
                   └────────────┘        └────────────┘ └──────────┘
```

### ECS Fargate 部署

最简单的容器化部署方式,无需管理节点。

#### 任务定义

```json
{
  "family": "copcon-server",
  "networkMode": "awsvpc",
  "requiresCompatibilities": ["FARGATE"],
  "cpu": "1024",
  "memory": "2048",
  "containerDefinitions": [
    {
      "name": "server",
      "image": "ghcr.io/copcon/copcon-server:v2.0.0",
      "essential": true,
      "portMappings": [
        { "containerPort": 8080, "protocol": "tcp" }
      ],
      "environment": [
        { "name": "CONFIG_PATH", "value": "/app/config.yaml" },
        { "name": "DATABASE_HOST", "value": "copcon-db.xxxxxx.region.rds.amazonaws.com" },
        { "name": "DATABASE_PORT", "value": "5432" },
        { "name": "DATABASE_USER", "value": "copcon" },
        { "name": "DATABASE_DBNAME", "value": "copcon" },
        { "name": "QDRANT_HOST", "value": "copcon-qdrant.xxxxxx.region.elastic-cloud.com" },
        { "name": "QDRANT_PORT", "value": "6333" }
      ],
      "secrets": [
        { "name": "DATABASE_PASSWORD", "valueFrom": "arn:aws:secretsmanager:region:account:secret:copcon/db-password" },
        { "name": "OPENAI_API_KEY", "valueFrom": "arn:aws:secretsmanager:region:account:secret:copcon/openai-key" }
      ],
      "healthCheck": {
        "command": ["CMD-SHELL", "wget -q --spider http://localhost:8080/health || exit 1"],
        "interval": 15,
        "timeout": 5,
        "retries": 3,
        "startPeriod": 10
      },
      "logConfiguration": {
        "logDriver": "awslogs",
        "options": {
          "awslogs-group": "/copcon/server",
          "awslogs-region": "ap-northeast-1",
          "awslogs-stream-prefix": "ecs"
        }
      }
    }
  ]
}
```

#### 服务配置

```bash
aws ecs create-service \
  --cluster copcon-cluster \
  --service-name copcon-server \
  --task-definition copcon-server \
  --desired-count 3 \
  --launch-type FARGATE \
  --network-configuration "awsvpcConfiguration={subnets=[subnet-xxx,subnet-yyy],securityGroups=[sg-xxx],assignPublicIp=DISABLED}" \
  --load-balancers "targetGroupArn=arn:aws:elasticloadbalancing:...,containerName=server,containerPort=8080"
```

### RDS PostgreSQL

用托管数据库替代集群内 PostgreSQL:

```bash
aws rds create-db-instance \
  --db-instance-identifier copcon-db \
  --db-instance-class db.t4g.medium \
  --engine postgres \
  --engine-version 15.4 \
  --master-username copcon \
  --master-user-password "$(aws secretsmanager get-random-password --password-length 32 --query RandomPassword --output text)" \
  --allocated-storage 50 \
  --storage-type gp3 \
  --vpc-security-group-ids sg-xxx \
  --db-subnet-group-name copcon-db-subnet \
  --backup-retention-period 7 \
  --multi-az
```

关键配置:
- **gp3 存储**: 比 gp2 性价比更高,基线 3000 IOPS
- **Multi-AZ**: 生产必须,自动故障切换
- **备份保留**: 至少 7 天,支持时间点恢复
- **t4g 实例**: Arm 架构,比 x86 便宜约 20%

### Qdrant on ECS

AWS 没有原生 Qdrant 托管服务,用 ECS 运行:

```bash
# 或使用 Qdrant Cloud
# https://cloud.qdrant.io/
```

如果使用 Qdrant Cloud,连接配置改为:

```yaml
qdrant:
  host: "xxxxx.aws.cloud.qdrant.io"
  port: 6333
  api_key: "${QDRANT_API_KEY}"
```

### 使用 Amazon Bedrock 替代 OpenAI

如果公司策略要求走 AWS 内网:

```yaml
openai:
  api_key: "${BEDROCK_API_KEY}"
  base_url: "https://bedrock-runtime.us-east-1.amazonaws.com"
  model: "anthropic.claude-3-sonnet"
```

### 成本估算 (月)

| 资源 | 规格 | 月成本(约) |
|------|------|-----------|
| ECS Fargate | 3 x 1vCPU/2GB | $75 |
| RDS PostgreSQL | db.t4g.medium Multi-AZ | $100 |
| ALB | 1 个 | $25 |
| CloudWatch | 日志 + 指标 | $10 |
| 数据传输 | ~100GB | $9 |
| **合计** | | **~$220/月** |

---

## Google Cloud

### 推荐架构

```
┌────────────┐     ┌──────────────┐     ┌────────────────┐
│ Cloud CDN  │────▶│  Cloud Load  │────▶│  Cloud Run     │
│            │     │  Balancing   │     │  copcon-server │
└────────────┘     └──────────────┘     └───────┬────────┘
                                                 │
                          ┌──────────────────────┼──────────────┐
                          ▼                      ▼              ▼
                   ┌────────────┐        ┌────────────┐ ┌──────────┐
                   │ Cloud SQL  │        │ Memorystore│ │ Vertex   │
                   │ PostgreSQL │        │ (Redis)    │ │ AI (LLM) │
                   └────────────┘        └────────────┘ └──────────┘
```

### Cloud Run 部署

最轻量的部署方式,按请求付费,自动扩缩到零:

```bash
gcloud run deploy copcon-server \
  --image ghcr.io/copcon/copcon-server:v2.0.0 \
  --platform managed \
  --region asia-east1 \
  --port 8080 \
  --cpu 2 \
  --memory 1Gi \
  --min-instances 1 \
  --max-instances 10 \
  --set-env-vars "CONFIG_PATH=/app/config.yaml" \
  --set-env-vars "DATABASE_HOST=/cloudsql/copcon-project:asia-east1:copcon-db" \
  --set-secrets "DATABASE_PASSWORD=copcon-db-password:latest" \
  --set-secrets "OPENAI_API_KEY=copcon-openai-key:latest" \
  --add-cloudsql-instances copcon-project:asia-east1:copcon-db \
  --allow-unauthenticated
```

#### Cloud Run 注意事项

- SSE 长连接受 Cloud Run 请求超时限制,默认 60 秒,最大 3600 秒
- 设置 `--timeout=300` 给 LLM 响应留余量
- `--min-instances 1` 避免冷启动(有费用)
- Cloud Run 通过 Unix socket 连接 Cloud SQL,无需公网

### GKE 部署

需要更多控制力时选择 GKE:

```bash
# 创建 Autopilot 集群
gcloud container clusters create-auto copcon-cluster \
  --region asia-east1

# 然后使用标准 Kubernetes 清单部署
kubectl apply -f k8s/
```

GKE Autopilot 自动管理节点,按 Pod 资源计费。

### Cloud SQL PostgreSQL

```bash
gcloud sql instances create copcon-db \
  --database-version POSTGRES_15 \
  --tier db-custom-2-8192 \
  --region asia-east1 \
  --availability-type regional \
  --storage-type SSD \
  --storage-size 50GB \
  --storage-auto-increase \
  --backup-start-time 03:00 \
  --enable-point-in-time-recovery \
  --maintenance-window-day SUN \
  --maintenance-window-hour 04

# 创建数据库
gcloud sql databases create copcon --instance copcon-db

# 创建用户
gcloud sql users create copcon --instance copcon-db --password "$(openssl rand -base64 24)"
```

### Vertex AI 替代 OpenAI

```yaml
openai:
  api_key: "${VERTEX_API_KEY}"
  base_url: "https://us-central1-aiplatform.googleapis.com/v1"
  model: "gemini-2.5-pro"
```

### 成本估算 (月)

| 资源 | 规格 | 月成本(约) |
|------|------|-----------|
| Cloud Run | 1-10 实例 | $30-100 |
| Cloud SQL | 2 vCPU/8GB HA | $120 |
| Cloud Load Balancing | 1 个 | $25 |
| Cloud Logging | ~50GB | $5 |
| **合计** | | **~$180-250/月** |

---

## Azure

### 推荐架构

```
┌────────────┐     ┌──────────────┐     ┌────────────────┐
│ Front Door │────▶│ App Gateway  │────▶│  Container Apps│
│ (CDN/WAF)  │     │ (Ingress)    │     │  copcon-server │
└────────────┘     └──────────────┘     └───────┬────────┘
                                                 │
                          ┌──────────────────────┼──────────────┐
                          ▼                      ▼              ▼
                   ┌────────────┐        ┌────────────┐ ┌──────────┐
                   │ Azure DB   │        │ Azure      │ │ Azure    │
                   │ PostgreSQL │        │ Cache      │ │ OpenAI   │
                   └────────────┘        └────────────┘ └──────────┘
```

### Azure Container Apps

最轻量的容器化部署:

```bash
# 创建环境
az containerapp env create \
  --name copcon-env \
  --resource-group copcon-rg \
  --location eastasia

# 部署 CopCon
az containerapp create \
  --name copcon-server \
  --resource-group copcon-rg \
  --environment copcon-env \
  --image ghcr.io/copcon/copcon-server:v2.0.0 \
  --target-port 8080 \
  --ingress external \
  --cpu 1 \
  --memory 1Gi \
  --min-replicas 1 \
  --max-replicas 10 \
  --env-vars \
    "CONFIG_PATH=/app/config.yaml" \
    "DATABASE_HOST=copcon-db.postgres.database.azure.com" \
  --secrets \
    "db-password=<YOUR_DB_PASSWORD>" \
    "openai-key=<YOUR_OPENAI_KEY>" \
  --set-env-vars \
    "DATABASE_PASSWORD=secretref:db-password" \
    "OPENAI_API_KEY=secretref:openai-key"
```

### AKS 部署

需要 Kubernetes 全部功能时选择 AKS:

```bash
az aks create \
  --name copcon-aks \
  --resource-group copcon-rg \
  --node-count 3 \
  --node-vm-size Standard_D2s_v5 \
  --enable-managed-identity \
  --enable-addons monitoring \
  --generate-ssh-keys

# 获取凭据
az aks get-credentials --name copcon-aks --resource-group copcon-rg

# 部署
kubectl apply -f k8s/
```

### Azure Database for PostgreSQL

```bash
az postgres flexible-server create \
  --name copcon-db \
  --resource-group copcon-rg \
  --location eastasia \
  --admin-user copcon \
  --admin-password "$(openssl rand -base64 24)" \
  --sku-name Standard_D2s_v3 \
  --tier GeneralPurpose \
  --storage-size 50 \
  --backup-retention 7 \
  --high-availability ZoneRedundant

# 创建数据库
az postgres flexible-server db create \
  --server-name copcon-db \
  --resource-group copcon-rg \
  --database-name copcon
```

### Azure OpenAI Service

Azure 上最自然的 LLM 选择:

```yaml
openai:
  api_key: "${AZURE_OPENAI_API_KEY}"
  base_url: "https://copcon-ai.openai.azure.com/openai/deployments/gpt-4o"
  model: "gpt-4o"
```

Azure OpenAI 提供企业级合规和数据隐私保障,数据不出 Azure 区域。

### 成本估算 (月)

| 资源 | 规格 | 月成本(约) |
|------|------|-----------|
| Container Apps | 1-10 实例 | $30-80 |
| Azure DB PostgreSQL | Standard_D2s_v3 HA | $140 |
| Application Gateway | WAF v2 | $200 |
| Azure OpenAI | 按用量 | 变动 |
| **合计** | | **~$370+/月** |

---

## 托管服务选择建议

| 组件 | 自建 vs 托管 | 推荐方案 |
|------|-------------|---------|
| CopCon Server | 容器托管 | ECS/Cloud Run/Container Apps |
| PostgreSQL | **托管优先** | RDS/Cloud SQL/Azure DB |
| Qdrant | 托管优先 | Qdrant Cloud (三大云均可用) |
| LLM API | **托管优先** | OpenAI/Bedrock/Vertex/Azure OpenAI |
| Redis 缓存 | 托管优先 | ElastiCache/Memorystore/Azure Cache |
| TLS 证书 | **托管优先** | ACM/Managed Certificate/Let's Encrypt |

**经验法则: 数据库和密钥管理用托管服务。CopCon 本身是无状态的,跑在容器平台最省心。**

## 通用最佳实践

### 1. 密钥管理

不要把 API Key 和数据库密码写进配置文件或容器镜像。

| 云 | 方案 |
|----|------|
| AWS | Secrets Manager + IAM Role |
| GCP | Secret Manager + Service Account |
| Azure | Key Vault + Managed Identity |

容器通过 IAM 角色自动获取密钥,代码中无需硬编码。

### 2. 网络安全

- CopCon 服务放在私有子网,不暴露公网 IP
- 通过 Load Balancer 或 API Gateway 对外
- 数据库只允许 CopCon 子网访问(Security Group / Firewall Rule)
- LLM API 出站走 NAT Gateway

### 3. 日志和监控

| 云 | 日志 | 指标 | 追踪 |
|----|------|------|------|
| AWS | CloudWatch Logs | CloudWatch Metrics | X-Ray |
| GCP | Cloud Logging | Cloud Monitoring | Cloud Trace |
| Azure | Log Analytics | Azure Monitor | Application Insights |

容器日志自动采集到云日志服务。CopCon 的 `logging` Hook 可补充结构化日志。

### 4. 成本优化

| 策略 | 预期节省 |
|------|---------|
| 非 7x24 工作负载用 Cloud Run / Container Apps(缩到零) | 40-60% |
| RDS/Cloud SQL 使用 Reserved Instance | 30-50% |
| Graviton/Arm 实例(t4g, Dpsv5) | 15-20% |
| 开发环境使用单 AZ,小规格 | 50%+ |
| 存储用 gp3 / SSD 自动扩容 | 10-20% |
| Spot/Preemptible 实例跑 Qdrant | 60-80% |

### 5. 多区域部署

如果需要跨区域高可用:

1. 数据库配置跨区域只读副本
2. 各区域独立部署 CopCon 服务
3. 主区域写入,副本区域读取
4. 使用全局负载均衡(Route53/Cloud CDN/Cloud Load Balancing)做故障切换

注意: CopCon 的 SSE 会话是有状态的(活跃 ChatContext 在内存中),故障切换会丢失进行中的对话。客户端需要实现重试逻辑。

## 下一步

- [Kubernetes 部署](kubernetes.md)
- [生产检查清单](production-checklist.md)
- [备份与恢复](backups.md)
