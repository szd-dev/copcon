# 监控与可观测性

本指南说明如何监控 CopCon 生产环境,包括指标采集、仪表盘、告警规则、日志聚合和分布式追踪。

## 可观测性三支柱

```
┌──────────────────────────────────────────────────┐
│                  可观测性体系                      │
│                                                   │
│   ┌──────────┐  ┌──────────┐  ┌──────────────┐  │
│   │ Metrics  │  │  Logs    │  │   Traces     │  │
│   │ 指标     │  │  日志    │  │   追踪       │  │
│   │          │  │          │  │              │  │
│   │ Prometheus│  │  Loki    │  │  Jaeger/     │  │
│   │          │  │  ELK     │  │  Tempo       │  │
│   └──────────┘  └──────────┘  └──────────────┘  │
│                                                   │
│              ┌──────────────┐                     │
│              │  Grafana     │                     │
│              │  统一可视化  │                     │
│              └──────────────┘                     │
└──────────────────────────────────────────────────┘
```

## 指标采集

### Prometheus

CopCon 暴露 Prometheus 指标端点(默认 `/metrics`,端口 8080)。

#### 采集配置

```yaml
# prometheus.yml
scrape_configs:
  - job_name: copcon-server
    scrape_interval: 15s
    metrics_path: /metrics
    static_configs:
      - targets:
          - copcon-server:8080
    # Kubernetes 服务发现
    # kubernetes_sd_configs:
    #   - role: pod
    #     namespaces:
    #       names: [copcon]
    # relabel_configs:
    #   - source_labels: [__meta_kubernetes_pod_label_app_kubernetes_io_name]
    #     action: keep
    #     regex: copcon-server
```

### OpenTelemetry

如果团队已使用 OTel,可通过 OTel Collector 统一采集:

```yaml
# otel-collector-config.yaml
receivers:
  prometheus:
    config:
      scrape_configs:
        - job_name: copcon-server
          static_configs:
            - targets: [copcon-server:8080]

exporters:
  prometheusremotewrite:
    endpoint: http://prometheus:9090/api/v1/write
  otlp:
    endpoint: jaeger:4317

service:
  pipelines:
    metrics:
      receivers: [prometheus]
      exporters: [prometheusremotewrite]
    traces:
      receivers: [otlp]
      exporters: [otlp]
```

## 关键性能指标 (KPI)

### 请求指标

| 指标 | 含义 | 警戒值 | 危险值 |
|------|------|--------|--------|
| `http_requests_total` | 请求总数 | - | - |
| `http_request_duration_seconds` | 请求延迟 P50/P95/P99 | P95 > 500ms | P99 > 2s |
| `http_requests_failed_total` | 失败请求数 | > 1/min | > 10/min |
| `http_requests_by_status` | 按 HTTP 状态码分组 | 5xx > 1% | 5xx > 5% |

### SSE 连接指标

| 指标 | 含义 | 警戒值 | 危险值 |
|------|------|--------|--------|
| `copcon_sse_connections_active` | 当前活跃 SSE 连接数 | > 500 | > 5000 |
| `copcon_sse_connections_total` | SSE 连接创建总数 | - | - |
| `copcon_sse_events_sent_total` | 发送的 SSE 事件数 | - | - |
| `copcon_sse_connection_duration_seconds` | SSE 连接持续时间 | - | - |

### LLM 调用指标

| 指标 | 含义 | 警戒值 | 危险值 |
|------|------|--------|--------|
| `copcon_llm_requests_total` | LLM 调用总数 | - | - |
| `copcon_llm_request_duration_seconds` | LLM 调用延迟 | P95 > 10s | P99 > 30s |
| `copcon_llm_requests_failed_total` | LLM 调用失败数 | > 5% | > 15% |
| `copcon_llm_tokens_total` | Token 消耗(输入/输出) | - | - |
| `copcon_llm_rate_limited_total` | 被限流次数 | > 0 | > 10/min |

### 工具调用指标

| 指标 | 含义 | 警戒值 | 危险值 |
|------|------|--------|--------|
| `copcon_tool_calls_total` | 工具调用总数 | - | - |
| `copcon_tool_call_duration_seconds` | 工具执行耗时 | P95 > 5s | P99 > 30s |
| `copcon_tool_calls_failed_total` | 工具调用失败数 | > 5% | > 20% |

### 系统资源指标

| 指标 | 含义 | 警戒值 | 危险值 |
|------|------|--------|--------|
| `process_cpu_seconds_total` | CPU 使用 | > 70% | > 90% |
| `process_resident_memory_bytes` | 内存占用 | > 70% limit | > 85% limit |
| `go_goroutines` | Goroutine 数量 | > 200 | > 1000 |
| `copcon_db_connections_active` | 活跃数据库连接 | > 80% pool | > 95% pool |

## Grafana 仪表盘

### CopCon 概览仪表盘

```json
{
  "title": "CopCon Overview",
  "panels": [
    {
      "title": "Request Rate",
      "type": "timeseries",
      "targets": [
        {
          "expr": "rate(http_requests_total{job='copcon-server'}[5m])"
        }
      ]
    },
    {
      "title": "Error Rate",
      "type": "stat",
      "targets": [
        {
          "expr": "rate(http_requests_failed_total{job='copcon-server'}[5m]) / rate(http_requests_total{job='copcon-server'}[5m])"
        }
      ],
      "thresholds": {
        "steps": [
          { "value": 0.01, "color": "green" },
          { "value": 0.05, "color": "yellow" },
          { "value": 0.1, "color": "red" }
        ]
      }
    },
    {
      "title": "Active SSE Connections",
      "type": "gauge",
      "targets": [
        {
          "expr": "copcon_sse_connections_active"
        }
      ]
    },
    {
      "title": "LLM Latency (P95)",
      "type": "timeseries",
      "targets": [
        {
          "expr": "histogram_quantile(0.95, rate(copcon_llm_request_duration_seconds_bucket[5m]))"
        }
      ]
    },
    {
      "title": "Token Usage",
      "type": "timeseries",
      "targets": [
        {
          "expr": "rate(copcon_llm_tokens_total{direction='input'}[1h])",
          "legendFormat": "Input"
        },
        {
          "expr": "rate(copcon_llm_tokens_total{direction='output'}[1h])",
          "legendFormat": "Output"
        }
      ]
    },
    {
      "title": "Goroutines",
      "type": "timeseries",
      "targets": [
        {
          "expr": "go_goroutines{job='copcon-server'}"
        }
      ]
    },
    {
      "title": "Memory Usage",
      "type": "timeseries",
      "targets": [
        {
          "expr": "process_resident_memory_bytes{job='copcon-server'}"
        }
      ]
    },
    {
      "title": "DB Connection Pool",
      "type": "gauge",
      "targets": [
        {
          "expr": "copcon_db_connections_active / copcon_db_connections_max * 100"
        }
      ]
    }
  ]
}
```

### 导入现成仪表盘

```bash
# Go Runtime 仪表盘(通用)
grafana-cli admin import-dashboard --uid go-runtime

# 导入 JSON
# 在 Grafana UI 中: Dashboards → Import → Upload JSON
```

## 告警规则

### Prometheus AlertManager 规则

```yaml
# copcon-alerts.yml
groups:
  - name: copcon-server
    rules:
      # 进程级告警
      - alert: CopConDown
        expr: up{job="copcon-server"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "CopCon server is down"
          description: "Instance {{ $labels.instance }} has been down for more than 1 minute."

      - alert: CopConHighMemory
        expr: process_resident_memory_bytes{job="copcon-server"} > 1.5 * 1024 * 1024 * 1024
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "CopCon memory usage high"
          description: "Memory usage {{ $value | humanize }}B exceeds 1.5GB."

      - alert: CopConGoroutineLeak
        expr: go_goroutines{job="copcon-server"} > 500
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Goroutine count suspiciously high"
          description: "{{ $value }} goroutines, possible leak."

      # 请求级告警
      - alert: CopConHighErrorRate
        expr: |
          rate(http_requests_failed_total{job="copcon-server"}[5m])
          /
          rate(http_requests_total{job="copcon-server"}[5m])
          > 0.05
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Error rate above 5%"
          description: "Current error rate: {{ $value | humanizePercentage }}."

      - alert: CopConHighLatency
        expr: |
          histogram_quantile(0.95,
            rate(http_request_duration_seconds_bucket{job="copcon-server"}[5m])
          ) > 2
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "P95 latency above 2 seconds"
          description: "Current P95: {{ $value }}s."

      # LLM 告警
      - alert: CopConLLMHighFailureRate
        expr: |
          rate(copcon_llm_requests_failed_total{job="copcon-server"}[5m])
          /
          rate(copcon_llm_requests_total{job="copcon-server"}[5m])
          > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "LLM API failure rate above 10%"

      - alert: CopConLLMRateLimited
        expr: rate(copcon_llm_rate_limited_total{job="copcon-server"}[5m]) > 0
        for: 2m
        labels:
          severity: info
        annotations:
          summary: "LLM API rate limiting detected"

      # 数据库告警
      - alert: CopConDBPoolExhaustion
        expr: |
          copcon_db_connections_active{job="copcon-server"}
          /
          copcon_db_connections_max{job="copcon-server"}
          > 0.9
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "DB connection pool near exhaustion"
          description: "{{ $value | humanizePercentage }} of pool in use."
```

### 告警路由

```yaml
# alertmanager.yml
route:
  group_by: [alertname, job]
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 4h
  receiver: slack

  routes:
    - match:
        severity: critical
      receiver: pagerduty
      repeat_interval: 15m

receivers:
  - name: slack
    slack_configs:
      - api_url: https://hooks.slack.com/services/xxx
        channel: "#copcon-alerts"

  - name: pagerduty
    pagerduty_configs:
      - service_key: <your-key>
```

## 日志聚合

### 结构化日志

CopCon 使用 `slog` 输出结构化日志。生产环境推荐 JSON 格式:

```json
{
  "time": "2025-01-15T10:30:00Z",
  "level": "INFO",
  "msg": "chat request processed",
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "agent_id": "assistant",
  "model": "gpt-4o",
  "duration_ms": 2340,
  "tokens_in": 150,
  "tokens_out": 420,
  "tool_calls": 2
}
```

### Loki 日志查询

```logql
# 所有错误日志
{job="copcon-server"} |= "level=ERROR"

# 特定会话的日志
{job="copcon-server"} | json | session_id="550e8400-..."

# LLM 调用失败的日志
{job="copcon-server"} |= "llm" |= "error"

# 慢请求(>5s)
{job="copcon-server"} | json | duration_ms > 5000
```

### ELK 日志查询

```kql
# 错误日志
level: "ERROR" AND job: "copcon-server"

# 特定会话
session_id: "550e8400-..."

# LLM 限流
message: "rate_limited" AND job: "copcon-server"
```

## 分布式追踪

### 配置 OpenTelemetry

CopCon 的 `tracing` Hook 支持将追踪数据发送到 OTel Collector:

```yaml
# config.yaml
hooks:
  - name: "tracing"
    type: "opentelemetry"
    enabled: true
    parameters:
      endpoint: "http://otel-collector:4318"
      service_name: "copcon-server"
      sampling_rate: 0.1  # 生产环境 10% 采样
```

### 追踪数据流

一个完整的聊天请求追踪:

```
[Chat Request]
  ├── [Pre-Process Hooks]
  │   └── [Memory Search] - 15ms
  ├── [LLM Call] - 1800ms
  │   ├── [Token 1] - 200ms (first token)
  │   └── [Stream Complete] - 1600ms
  ├── [Tool Call: code_executor] - 500ms
  │   └── [Sandbox Execute] - 450ms
  ├── [LLM Call (continuation)] - 800ms
  └── [Post-Process Hooks]
      └── [Memory Save] - 20ms
```

### Jaeger 查询

```
# 按 session_id 搜索
service=copcon-server tag=session_id:550e8400-...

# 慢请求
service=copcon-server minDuration=5s

# 失败请求
service=copcon-server tags={"error":true}
```

## 监控即代码

所有监控配置应纳入版本控制:

```
monitoring/
├── prometheus/
│   ├── prometheus.yml         # 采集配置
│   └── alerts/
│       └── copcon-alerts.yml  # 告警规则
├── alertmanager/
│   └── alertmanager.yml       # 告警路由
├── grafana/
│   └── dashboards/
│       └── copcon-overview.json
├── loki/
│   └── loki-config.yml
└── otel-collector/
    └── config.yml
```

### 用 Kubernetes 部署监控栈

```bash
# 使用 kube-prometheus-stack Helm Chart
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm install monitoring prometheus-community/kube-prometheus-stack \
  --namespace monitoring \
  --create-namespace \
  --set grafana.adminPassword=your-password

# 导入 CopCon 告警规则
kubectl apply -f monitoring/prometheus/alerts/copcon-alerts.yml
```

## 运维操作

### 检查服务健康

```bash
# 快速检查
curl -s http://localhost:8080/health | jq .

# 检查指标
curl -s http://localhost:8080/metrics | grep copcon_

# 检查活跃连接
curl -s http://localhost:8080/metrics | grep copcon_sse_connections_active
```

### 排查性能问题

1. 查看 P95 延迟趋势(Grafana)
2. 查看 LLM API 调用延迟(是否是 LLM 侧慢)
3. 查看数据库连接池使用率
4. 查看 Goroutine 数量(是否泄漏)
5. 查看内存趋势(是否持续增长)

详见 [故障排查指南](troubleshooting.md)。

## 下一步

- [生产检查清单](production-checklist.md)
- [故障排查](troubleshooting.md)
- [扩缩容](scaling.md)