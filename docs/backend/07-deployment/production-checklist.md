# 生产环境检查清单

上线前逐一确认以下项目,确保 CopCon 在生产环境稳定运行。

## 部署前检查

### 基础设施

- [ ] PostgreSQL 15+ 可用且连接正常
- [ ] 数据库初始化完成(sessions/messages 表、索引、触发器)
- [ ] Qdrant 1.17+ 可用(或确认记忆功能不需要)
- [ ] Qdrant collection 已创建(agent_memory, cosine, 1536 维)
- [ ] LLM API 可达且 Key 有效(curl 测试通过)
- [ ] 网络拓扑确认: CopCon → PostgreSQL, CopCon → Qdrant, CopCon → LLM API
- [ ] DNS 解析正常(所有服务主机名可达)
- [ ] TLS 证书已配置(或反向代理已处理)
- [ ] 端口未被占用(CopCon 默认 8080, 或配置的端口)

### 配置

- [ ] `config.yaml` 生产配置已就绪
- [ ] API Key 通过环境变量注入,未硬编码在配置文件中
- [ ] 数据库密码已替换为强密码(非 changeme/agent123)
- [ ] Agent 定义中 model 和 tools 符合业务需求
- [ ] system_prompt 已针对生产场景调整
- [ ] 日志级别设置为 `info`(非 debug,避免敏感数据泄露)
- [ ] CORS 配置已限制为实际需要的域名
- [ ] 配置文件权限已限制(600,仅运行用户可读)

### 资源评估

- [ ] CPU: 至少 2 核(推荐 4 核+),SSE 连接对单核敏感
- [ ] 内存: 至少 2GB(推荐 8GB),含数据库连接池开销
- [ ] 磁盘: 至少 50GB SSD,数据库 I/O 对延迟影响大
- [ ] 网络: LLM API 出口带宽和延迟已评估
- [ ] PostgreSQL 独立部署,不与 CopCon 共用主机
- [ ] 连接池大小已根据并发量配置(默认 100 最大,10 空闲)

## 安全加固

### 网络安全

- [ ] CopCon 服务不直接暴露公网,通过反向代理(Nginx/Caddy/ALB)
- [ ] PostgreSQL 只允许 CopCon 子网/IP 访问
- [ ] Qdrant 只允许 CopCon 子网/IP 访问
- [ ] 出站 LLM API 调用走 NAT,不绕过防火墙
- [ ] 如在 Kubernetes 中,NetworkPolicy 已配置
- [ ] SSH/管理端口不暴露公网

### 进程安全

- [ ] CopCon 以非 root 用户运行(专用 copcon 用户)
- [ ] systemd 配置含安全加固项(NoNewPrivileges, ProtectSystem, PrivateTmp)
- [ ] 容器以非 root 用户运行(Dockerfile 中 USER copcon)
- [ ] 容器使用 `--read-only` 根文件系统
- [ ] 容器已 `--cap-drop=ALL`,仅保留必要能力
- [ ] 不挂载 Docker socket 到容器内

### 数据安全

- [ ] API Key 存储在密钥管理服务(Secrets Manager/Vault),非环境变量直传
- [ ] 数据库启用 SSL 连接(sslmode=require)
- [ ] 备份文件加密存储
- [ ] 日志中不含 API Key 和用户敏感数据
- [ ] 会话数据访问有权限控制(或规划中)
- [ ] 配置文件不在 Git 仓库中(或 .gitignore 已排除)

### 工具安全

- [ ] 仅启用业务需要的工具,未使用的工具从 Agent 配置中移除
- [ ] `shell_executor` 在生产环境评估是否需要(安全风险高)
- [ ] `code_executor` 使用 Docker 镜像隔离(network=none)
- [ ] 工具执行超时已配置(默认 60s)
- [ ] 工具输出大小已限制(防止过大输出耗尽内存)

## 性能基线

### 基准测试

上线前完成以下基准测试,记录结果作为基线:

- [ ] 健康检查响应时间 < 10ms
- [ ] 单会话创建延迟 < 50ms
- [ ] 单消息发送(LLM 直连)首 token 延迟 < 200ms
- [ ] SSE 流式传输无中断(持续 5 分钟测试)
- [ ] 并发 100 会话创建成功率 > 99%
- [ ] 并发 50 活跃 SSE 连接稳定性(30 分钟)
- [ ] 内存占用基线: 空载 ~200MB, 50 连接 ~500MB

### 负载测试

- [ ] 模拟峰值负载的 2 倍压力测试通过
- [ ] 测试 LLM API 限流场景下的表现(429 响应处理)
- [ ] 测试数据库连接池耗尽场景
- [ ] 测试 Qdrant 不可用时记忆 Hook 静默跳过
- [ ] 长时间运行测试(24 小时+)无内存泄漏

测试工具推荐:
- [wrk](https://github.com/wg/wrk) - HTTP 压测
- [k6](https://k6.io/) - 脚本化负载测试
- 自定义脚本模拟 SSE 长连接

## 监控和告警

### 指标采集

- [ ] Prometheus 指标端点可用(/metrics)
- [ ] 关键指标已采集:
  - 请总数和错误率
  - 活跃 SSE 连接数
  - LLM API 调用延迟和成功率
  - 数据库查询延迟
  - 进程 CPU/内存使用率

### 告警规则

- [ ] CopCon 进程停止 → 立即告警
- [ ] 健康检查连续失败 3 次 → 告警
- [ ] 错误率 > 5% → 告警
- [ ] SSE 连接数异常增长(可能的 DDoS) → 告警
- [ ] LLM API 调用失败率 > 10% → 告警
- [ ] PostgreSQL 连接池接近上限 → 告警
- [ ] 磁盘使用 > 80% → 告警
- [ ] 内存使用 > 85% → 告警

### 日志

- [ ] 结构化日志输出(JSON 格式)
- [ ] 日志级别正确(info,非 debug)
- [ ] 日志轮转配置(50MB x 5,或类似)
- [ ] 日志不包含 API Key 和用户隐私数据
- [ ] 日志聚合服务可用(Loki/ELK/云日志服务)

### 健康检查

- [ ] HTTP 健康检查端点 /health 已配置
- [ ] 健康检查集成到:
  - systemd (可选)
  - Docker/Docker Compose healthcheck
  - Kubernetes liveness/readiness probe
  - 负载均衡器健康检查

## 备份验证

### 数据库

- [ ] PostgreSQL 自动备份已配置(每日)
- [ ] 备份保留至少 7 天
- [ ] 已验证备份文件可成功恢复
- [ ] 时间点恢复(PITR)功能已启用(如果 RDS/Cloud SQL)
- [ ] 恢复时间目标(RTO)已确认: < 1 小时
- [ ] 恢复点目标(RPO)已确认: < 24 小时(或更低)

### Qdrant

- [ ] Qdrant 数据快照已配置(定期)
- [ ] 快照存储到持久卷或 S3
- [ ] 已验证快照可成功恢复

### 配置

- [ ] config.yaml 已备份(不在 Git 中的版本)
- [ ] .env 文件已安全备份
- [ ] Kubernetes Secret 有备份策略(或可从密钥管理服务重建)

## 灾难恢复

- [ ] 恢复流程文档已编写并团队可访问
- [ ] 恢复流程至少演练过一次
- [ ] 数据库故障切换已测试(Multi-AZ/主从切换)
- [ ] CopCon 全部实例宕机后的重启流程已确认
- [ ] 配置文件丢失后的重建流程已确认

## Go-Live 检查

上线当天最后确认:

- [ ] 所有预部署检查项已完成
- [ ] 监控仪表盘已上线且可查看
- [ ] 告警通道已验证(邮件/Slack/PagerDuty 能收到)
- [ ] 备份已完成首次全量备份
- [ ] 回滚方案已确认(上一版本二进制/镜像已保留)
- [ ] 团队有人值班,熟悉恢复流程
- [ ] 上线窗口已通知相关团队
- [ ] DNS 切换或流量切换方案已确认
- [ ] 首次上线后 30 分钟内密切观察错误率和延迟
- [ ] 确认无调试日志泄露敏感信息

## 上线后验证

上线后 24 小时内确认:

- [ ] 错误率在基线范围内(< 1%)
- [ ] SSE 连接无异常断开
- [ ] LLM API 调用成功率正常
- [ ] 内存使用稳定,无持续增长(泄漏迹象)
- [ ] 数据库连接数稳定
- [ ] 备份已自动执行
- [ ] 告警无误报

## 相关文档

- [安装指南](installation.md)
- [监控与可观测性](monitoring.md)
- [备份与恢复](backups.md)
- [故障排查](troubleshooting.md)