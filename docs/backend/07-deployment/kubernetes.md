# Kubernetes 部署指南

本指南提供 CopCon 在 Kubernetes 上的完整部署方案,包含清单文件、Helm Chart 和运维操作。

## 架构概览

```
                    ┌──────────────┐
                    │   Ingress    │
                    │  Controller  │
                    └──────┬───────┘
                           │
                    ┌──────▼───────┐
                    │   Service    │
                    │  ClusterIP   │
                    └──────┬───────┘
                           │
              ┌────────────┼────────────┐
              ▼            ▼            ▼
         ┌─────────┐ ┌─────────┐ ┌─────────┐
         │  Pod 1  │ │  Pod 2  │ │  Pod 3  │
         │ server  │ │ server  │ │ server  │
         └─────────┘ └─────────┘ └─────────┘
              │            │            │
              └────────────┼────────────┘
                           │
              ┌────────────┼────────────┐
              ▼                         ▼
     ┌──────────────────┐    ┌──────────────────┐
     │  PostgreSQL      │    │    Qdrant        │
     │  StatefulSet     │    │  StatefulSet     │
     │  (或外部 RDS)    │    │  (或外部托管)    │
     └──────────────────┘    └──────────────────┘
```

## 命名空间

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: copcon
  labels:
    app.kubernetes.io/part-of: copcon
```

## ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: copcon-config
  namespace: copcon
data:
  config.yaml: |
    server:
      port: "8080"
    database:
      host: "copcon-postgres"
      port: 5432
      user: "copcon"
      dbname: "copcon"
    openai:
      base_url: "https://api.openai.com/v1"
      model: "gpt-4o"
    qdrant:
      host: "copcon-qdrant"
      port: 6333
    default_agent_id: "assistant"
    agents:
      - id: "assistant"
        name: "Assistant"
        model: "gpt-4o"
        system_prompt: "You are a helpful assistant."
        tools: []
```

## Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: copcon-secrets
  namespace: copcon
type: Opaque
stringData:
  DATABASE_PASSWORD: "changeme"     # 生产环境用 sealed-secrets 或 Vault
  OPENAI_API_KEY: "sk-placeholder"  # 同上
```

生产环境不要把明文密钥提交到 Git。推荐工具:
- [Sealed Secrets](https://github.com/bitnami-labs/sealed-secrets)
- [External Secrets](https://external-secrets.io/)
- [HashiCorp Vault](https://www.vaultproject.io/)

## CopCon Server Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: copcon-server
  namespace: copcon
  labels:
    app.kubernetes.io/name: copcon-server
    app.kubernetes.io/component: api
spec:
  replicas: 3
  selector:
    matchLabels:
      app.kubernetes.io/name: copcon-server
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0  # 零停机更新
  template:
    metadata:
      labels:
        app.kubernetes.io/name: copcon-server
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8080"
        prometheus.io/path: "/metrics"
    spec:
      serviceAccountName: copcon-server
      terminationGracePeriodSeconds: 30

      # 反亲和: Pod 尽量分布到不同节点
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
            - weight: 100
              podAffinityTerm:
                labelSelector:
                  matchLabels:
                    app.kubernetes.io/name: copcon-server
                topologyKey: kubernetes.io/hostname

      containers:
        - name: server
          image: ghcr.io/copcon/copcon-server:v2.0.0
          ports:
            - containerPort: 8080
              name: http
              protocol: TCP

          env:
            - name: CONFIG_PATH
              value: "/app/config.yaml"
            - name: DATABASE_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: copcon-secrets
                  key: DATABASE_PASSWORD
            - name: OPENAI_API_KEY
              valueFrom:
                secretKeyRef:
                  name: copcon-secrets
                  key: OPENAI_API_KEY
            - name: DATABASE_HOST
              value: "copcon-postgres"
            - name: QDRANT_HOST
              value: "copcon-qdrant"

          volumeMounts:
            - name: config
              mountPath: /app/config.yaml
              subPath: config.yaml
              readOnly: true

          # 资源请求和限制
          resources:
            requests:
              cpu: "500m"
              memory: "256Mi"
            limits:
              cpu: "2"
              memory: "1Gi"

          # 存活探针: 进程是否还活着
          livenessProbe:
            httpGet:
              path: /health
              port: http
            initialDelaySeconds: 10
            periodSeconds: 15
            timeoutSeconds: 5
            failureThreshold: 3

          # 就绪探针: 是否能接收流量
          readinessProbe:
            httpGet:
              path: /health
              port: http
            initialDelaySeconds: 5
            periodSeconds: 10
            timeoutSeconds: 3
            failureThreshold: 3

          # 启动探针: 启动慢时给更多时间
          startupProbe:
            httpGet:
              path: /health
              port: http
            initialDelaySeconds: 0
            periodSeconds: 5
            failureThreshold: 12  # 最多等 60 秒

      volumes:
        - name: config
          configMap:
            name: copcon-config
```

## Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: copcon-server
  namespace: copcon
spec:
  type: ClusterIP
  selector:
    app.kubernetes.io/name: copcon-server
  ports:
    - port: 8080
      targetPort: http
      name: http
```

## Ingress

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: copcon-ingress
  namespace: copcon
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
    nginx.ingress.kubernetes.io/proxy-buffering: "off"       # SSE 必须
    nginx.ingress.kubernetes.io/proxy-read-timeout: "300"    # LLM 可能慢
    nginx.ingress.kubernetes.io/proxy-send-timeout: "300"
spec:
  ingressClassName: nginx
  tls:
    - hosts:
        - copcon.example.com
      secretName: copcon-tls
  rules:
    - host: copcon.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: copcon-server
                port:
                  name: http
```

`proxy-buffering: "off"` 是 SSE 流式响应的关键配置。缺少它会导致客户端收不到实时事件。

## PostgreSQL StatefulSet

如果不在集群内运行 PostgreSQL,跳过此节,改用外部 RDS。详见 [云服务商部署](cloud-providers.md)。

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: copcon-postgres
  namespace: copcon
spec:
  serviceName: copcon-postgres
  replicas: 1
  selector:
    matchLabels:
      app: copcon-postgres
  template:
    metadata:
      labels:
        app: copcon-postgres
    spec:
      containers:
        - name: postgres
          image: postgres:15-alpine
          ports:
            - containerPort: 5432
          env:
            - name: POSTGRES_USER
              value: "copcon"
            - name: POSTGRES_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: copcon-secrets
                  key: DATABASE_PASSWORD
            - name: POSTGRES_DB
              value: "copcon"
            - name: PGDATA
              value: /var/lib/postgresql/data/pgdata
          volumeMounts:
            - name: postgres-data
              mountPath: /var/lib/postgresql/data
          resources:
            requests:
              cpu: "500m"
              memory: "512Mi"
            limits:
              cpu: "2"
              memory: "2Gi"
          livenessProbe:
            exec:
              command: ["pg_isready", "-U", "copcon"]
            initialDelaySeconds: 30
            periodSeconds: 10
  volumeClaimTemplates:
    - metadata:
        name: postgres-data
      spec:
        accessModes: ["ReadWriteOnce"]
        storageClassName: standard  # 按集群配置调整
        resources:
          requests:
            storage: 20Gi
---
apiVersion: v1
kind: Service
metadata:
  name: copcon-postgres
  namespace: copcon
spec:
  type: ClusterIP
  selector:
    app: copcon-postgres
  ports:
    - port: 5432
      targetPort: 5432
```

## Qdrant StatefulSet

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: copcon-qdrant
  namespace: copcon
spec:
  serviceName: copcon-qdrant
  replicas: 1
  selector:
    matchLabels:
      app: copcon-qdrant
  template:
    metadata:
      labels:
        app: copcon-qdrant
    spec:
      containers:
        - name: qdrant
          image: qdrant/qdrant:v1.17.0
          ports:
            - containerPort: 6333
              name: http
            - containerPort: 6334
              name: grpc
          volumeMounts:
            - name: qdrant-data
              mountPath: /qdrant/storage
          resources:
            requests:
              cpu: "250m"
              memory: "256Mi"
            limits:
              cpu: "1"
              memory: "1Gi"
          livenessProbe:
            httpGet:
              path: /health
              port: http
            initialDelaySeconds: 10
            periodSeconds: 10
  volumeClaimTemplates:
    - metadata:
        name: qdrant-data
      spec:
        accessModes: ["ReadWriteOnce"]
        storageClassName: standard
        resources:
          requests:
            storage: 10Gi
---
apiVersion: v1
kind: Service
metadata:
  name: copcon-qdrant
  namespace: copcon
spec:
  type: ClusterIP
  selector:
    app: copcon-qdrant
  ports:
    - port: 6333
      targetPort: http
      name: http
    - port: 6334
      targetPort: grpc
      name: grpc
```

## Horizontal Pod Autoscaler

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: copcon-server-hpa
  namespace: copcon
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: copcon-server
  minReplicas: 2
  maxReplicas: 10
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: 80
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 60
      policies:
        - type: Pods
          value: 2
          periodSeconds: 60
    scaleDown:
      stabilizationWindowSeconds: 300  # 缩容保守,5 分钟稳定后才缩
      policies:
        - type: Pods
          value: 1
          periodSeconds: 120
```

## PodDisruptionBudget

确保滚动更新和节点维护时始终有足够副本:

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: copcon-server-pdb
  namespace: copcon
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: copcon-server
```

## NetworkPolicy

限制 Pod 间网络访问:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: copcon-server-netpol
  namespace: copcon
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: copcon-server
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: ingress-nginx
      ports:
        - port: 8080
  egress:
    - to:
        - podSelector:
            matchLabels:
              app: copcon-postgres
      ports:
        - port: 5432
    - to:
        - podSelector:
            matchLabels:
              app: copcon-qdrant
      ports:
        - port: 6333
    - to:
        - namespaceSelector: {}
          podSelector:
            matchLabels:
              k8s-app: kube-dns
      ports:
        - port: 53
          protocol: UDP
    # 允许出站到 LLM API (需要 CIDR 或 FQDN 策略)
```

## Helm Chart

如果你更偏好 Helm,项目提供了 Chart:

```bash
# 添加 Helm 仓库
helm repo add copcon https://copcon.github.io/charts
helm repo update

# 安装
helm install copcon copcon/copcon-server \
  --namespace copcon \
  --create-namespace \
  --set secrets.openaiApiKey=sk-your-key \
  --set secrets.databasePassword=your-password \
  --set server.replicas=3
```

### values.yaml 示例

```yaml
server:
  replicas: 3
  image:
    repository: ghcr.io/copcon/copcon-server
    tag: v2.0.0
    pullPolicy: IfNotPresent
  resources:
    requests:
      cpu: 500m
      memory: 256Mi
    limits:
      cpu: "2"
      memory: 1Gi

database:
  enabled: true    # 在集群内运行 PostgreSQL
  storage: 20Gi

qdrant:
  enabled: true    # 在集群内运行 Qdrant
  storage: 10Gi

ingress:
  enabled: true
  host: copcon.example.com
  tls: true

secrets:
  openaiApiKey: ""      # 必须设置
  databasePassword: ""  # 必须设置

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilization: 70
```

## 部署操作

### 首次部署

```bash
# 创建命名空间
kubectl create namespace copcon

# 应用所有清单
kubectl apply -f namespace.yaml
kubectl apply -f configmap.yaml
kubectl apply -f secret.yaml
kubectl apply -f postgres.yaml
kubectl apply -f qdrant.yaml
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
kubectl apply -f ingress.yaml
kubectl apply -f hpa.yaml
kubectl apply -f pdb.yaml

# 等待就绪
kubectl -n copcon rollout status deployment/copcon-server
```

### 查看状态

```bash
# Pod 状态
kubectl -n copcon get pods -l app.kubernetes.io/name=copcon-server

# 服务端点
kubectl -n copcon get svc

# 事件
kubectl -n copcon get events --sort-by='.lastTimestamp'

# 日志
kubectl -n copcon logs -l app.kubernetes.io/name=copcon-server -f
```

### 更新部署

```bash
# 更新镜像版本
kubectl -n copcon set image deployment/copcon-server \
  server=ghcr.io/copcon/copcon-server:v2.1.0

# 或修改 ConfigMap 后重启
kubectl -n copcon rollout restart deployment/copcon-server

# 查看滚动更新状态
kubectl -n copcon rollout status deployment/copcon-server
```

### 回滚

```bash
# 查看历史
kubectl -n copcon rollout history deployment/copcon-server

# 回滚到上一版本
kubectl -n copcon rollout undo deployment/copcon-server

# 回滚到指定版本
kubectl -n copcon rollout undo deployment/copcon-server --to-revision=2
```

## 初始化任务

数据库表由 GORM AutoMigrate 在服务器启动时自动创建,无需单独的初始化 Job。

Qdrant 集合初始化可以用 Job 完成:

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: copcon-init-qdrant
  namespace: copcon
spec:
  template:
    spec:
      restartPolicy: OnFailure
      containers:
        - name: init-qdrant
          image: curlimages/curl:latest
          command:
            - curl
            - -X
            - PUT
            - http://copcon-qdrant:6333/collections/agent_memory
            - -H
            - "Content-Type: application/json"
            - -d
            - '{"vectors": {"size": 1536, "distance": "Cosine"}}'
```

## 下一步

- [云服务商部署](cloud-providers.md)
- [监控与可观测性](monitoring.md)
- [扩缩容](scaling.md)
