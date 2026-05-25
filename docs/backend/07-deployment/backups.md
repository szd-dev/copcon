# 备份与灾难恢复

本指南说明 CopCon 数据的备份策略、操作步骤和恢复流程。

## 备份策略总览

CopCon 的数据分布在三个存储层,各有不同的备份需求:

| 存储 | 数据类型 | 丢失影响 | 备份优先级 |
|------|---------|---------|-----------|
| PostgreSQL | 会话、消息、任务 | 不可恢复,业务中断 | **最高** |
| Qdrant | 向量记忆 | 可重建(重新嵌入),代价大 | 高 |
| config.yaml | 配置和密钥 | 服务无法启动 | 高 |

## PostgreSQL 备份

### 方案一: pg_dump 全量备份

最简单可靠,适合中小规模。

```bash
#!/bin/bash
# backup-postgres.sh

BACKUP_DIR="/data/backups/postgres"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="$BACKUP_DIR/copcon_${TIMESTAMP}.sql.gz"

# 确保目录存在
mkdir -p $BACKUP_DIR

# 全量备份
PGPASSWORD=$DB_PASSWORD pg_dump \
  -h $DB_HOST \
  -p $DB_PORT \
  -U $DB_USER \
  -d copcon \
  --format=custom \
  --verbose \
  | gzip > $BACKUP_FILE

# 检查备份文件
if [ -f "$BACKUP_FILE" ] && [ $(stat -f%z "$BACKUP_FILE") -gt 0 ]; then
  echo "Backup successful: $BACKUP_FILE"
  echo "Size: $(du -h $BACKUP_FILE | cut -f1)"
else
  echo "Backup FAILED!"
  exit 1
fi

# 清理超过 30 天的备份
find $BACKUP_DIR -name "*.sql.gz" -mtime +30 -delete

echo "Old backups cleaned up"
```

设置 cron 每日执行:

```bash
# crontab -e
0 3 * * * /data/copcon/scripts/backup-postgres.sh >> /var/log/copcon-backup.log 2>&1
```

### 方案二: 自托管 PostgreSQL 持续归档

适合不允许任何数据丢失的场景。

```bash
# postgresql.conf
wal_level = replica
archive_mode = on
archive_command = 'cp %p /data/backups/postgres/wal/%f'
max_wal_senders = 3
```

同时做基础备份:

```bash
pg_basebackup \
  -h localhost \
  -U replication_user \
  -D /data/backups/postgres/base \
  --format=tar \
  --gzip \
  --progress \
  --write-recovery-conf
```

恢复时: 先还原基础备份,然后重放 WAL 日志到任意时间点。

### 方案三: 云托管数据库自动备份

RDS / Cloud SQL / Azure DB 都内置自动备份:

```bash
# AWS RDS: 自动备份 + PITR,无需手动操作
# 确认配置:
aws rds describe-db-instance \
  --db-instance-identifier copcon-db \
  --query 'DBInstance.{Backup:BackupRetentionPeriod, PITR:BackupRetentionPeriod>0}'

# Cloud SQL: 自动备份
gcloud sql instances describe copcon-db --format='value(backupRetentionPeriod)'

# Azure DB: 自动备份,默认保留 7 天
az postgres flexible-server show \
  --name copcon-db \
  --query "backupRetentionDays"
```

这些托管服务还支持时间点恢复(PITR),可以将数据库恢复到过去任意一分钟的状态。

### 手动快照(云平台)

```bash
# AWS RDS 快照
aws rds create-db-snapshot \
  --db-instance-identifier copcon-db \
  --db-snapshot-identifier copcon-manual-$(date +%Y%m%d)

# Cloud SQL 备份
gcloud sql backups create \
  --instance copcon-db \
  --description "manual-backup-$(date +%Y%m%d)"

# Azure 快照(通过 PITR)
# Azure DB Flexible Server 自动备份,PITR 可恢复到任意时间点
```

## Qdrant 备份

### 创建快照

```bash
#!/bin/bash
# backup-qdrant.sh

QDRANT_HOST="${QDRANT_HOST:-localhost}"
QDRANT_PORT="${QDRANT_PORT:-6333}"
COLLECTION="agent_memory"
BACKUP_DIR="/data/backups/qdrant"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

mkdir -p $BACKUP_DIR

# 创建快照
echo "Creating Qdrant snapshot..."
curl -X POST "http://$QDRANT_HOST:$QDRANT_PORT/collections/$COLLECTION/snapshots" \
  -H "Content-Type: application/json"

# 等待快照完成
sleep 5

# 下载快照文件
echo "Downloading snapshot..."
curl -s "http://$QDRANT_HOST:$QDRANT_PORT/collections/$COLLECTION/snapshots" | \
  jq -r '.result[0].name' > /tmp/snapshot_name.txt

SNAPSHOT_NAME=$(cat /tmp/snapshot_name.txt)
curl -o "$BACKUP_DIR/${COLLECTION}_${TIMESTAMP}.snapshot" \
  "http://$QDRANT_HOST:$QDRANT_PORT/collections/$COLLECTION/snapshots/$SNAPSHOT_NAME"

# 验证下载
if [ -f "$BACKUP_DIR/${COLLECTION}_${TIMESTAMP}.snapshot" ]; then
  SIZE=$(du -h "$BACKUP_DIR/${COLLECTION}_${TIMESTAMP}.snapshot" | cut -f1)
  echo "Snapshot downloaded: $SIZE"
else
  echo "Snapshot download FAILED!"
  exit 1
fi

# 清理旧快照
find $BACKUP_DIR -name "*.snapshot" -mtime +30 -delete

echo "Done!"
```

### Qdrant Cloud 备份

如果使用 Qdrant Cloud,快照功能通过管理界面或 API 触发。Qdrant Cloud 自带高可用和冗余存储,可适当降低备份频率。

### 从快照恢复

```bash
# 1. 上传快照到 Qdrant
curl -X PUT "http://$QDRANT_HOST:$QDRANT_PORT/collections/$COLLECTION/snapshots/upload" \
  -H "Content-Type: multipart/form-data" \
  -F "snapshot=@/data/backups/qdrant/agent_memory_20250115_030000.snapshot"

# 2. 从快照恢复 collection
curl -X PUT "http://$QDRANT_HOST:$QDRANT_PORT/collections/$COLLECTION/snapshots/recover" \
  -H "Content-Type: application/json" \
  -d '{"location": "agent_memory_20250115_030000.snapshot"}'

# 3. 验证恢复
curl "http://$QDRANT_HOST:$QDRANT_PORT/collections/$COLLECTION" | jq '.result.points_count'
```

## 配置备份

配置文件和密钥同样需要备份:

```bash
#!/bin/bash
# backup-config.sh

BACKUP_DIR="/data/backups/config"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

mkdir -p $BACKUP_DIR

# 备份配置文件
cp /etc/copcon/config.yaml "$BACKUP_DIR/config_${TIMESTAMP}.yaml"

# 备份环境变量文件(注意权限!)
cp /etc/copcon/env "$BACKUP_DIR/env_${TIMESTAMP}" 
chmod 600 "$BACKUP_DIR/env_${TIMESTAMP}"

# 备份 Kubernetes Secret(如果用 k8s)
kubectl -n copcon get secrets -o yaml > "$BACKUP_DIR/k8s-secrets_${TIMESTAMP}.yaml"

# 加密备份(推荐)
gpg --symmetric --cipher-algo AES256 \
  "$BACKUP_DIR/env_${TIMESTAMP}" \
  --output "$BACKUP_DIR/env_${TIMESTAMP}.gpg"

echo "Config backup done: $BACKUP_DIR"
```

## 备份验证

备份不验证等于没有备份。定期验证是必须的。

### 自动验证脚本

```bash
#!/bin/bash
# verify-backup.sh

BACKUP_DIR="/data/backups/postgres"
LATEST=$(ls -t $BACKUP_DIR/*.sql.gz | head -1)

echo "Verifying: $LATEST"

# 解压并检查内容
zcat $LATEST | head -5

# 尝试恢复到临时数据库
PGPASSWORD=test pg_restore \
  --list $LATEST 2>&1 | head -20

# 检查备份完整性
zcat $LATEST | grep -c "CREATE TABLE" && echo "Tables found"
zcat $LATEST | grep -c "COPY" && echo "Data sections found"

echo "Verification complete"
```

### 定期恢复演练

建议每季度做一次完整恢复演练:

1. 在隔离环境中创建测试数据库
2. 从备份恢复数据
3. 验证数据完整性(行数、关键字段)
4. 启动 CopCon 服务连接测试数据库
5. 验证 API 功能正常(创建会话、发送消息)
6. 记录恢复耗时

## 备份到远程存储

本地备份不够安全。需要将备份复制到异地存储。

### S3 (AWS)

```bash
# 同步备份到 S3
aws s3 sync /data/backups/postgres/ s3://copcon-backups/postgres/
aws s3 sync /data/backups/qdrant/ s3://copcon-backups/qdrant/
aws s3 sync /data/backups/config/ s3://copcon-backups/config/

# S3 生命周期策略(自动清理旧备份)
aws s3api put-bucket-lifecycleconfiguration \
  --bucket copcon-backups \
  --lifecycle-configuration file://s3-lifecycle.json
```

### GCS (Google Cloud)

```bash
gsutil -m rsync -r /data/backups/ gs://copcon-backups/
```

### Azure Blob Storage

```bash
az storage blob upload-batch \
  --destination backups \
  --source /data/backups/ \
  --account-name copconstorage
```

## 恢复流程

### PostgreSQL 完整恢复

```bash
#!/bin/bash
# restore-postgres.sh

BACKUP_FILE="/data/backups/postgres/copcon_20250115_030000.sql.gz"

# 1. 停止 CopCon 服务
sudo systemctl stop copcon

# 2. 丢弃旧数据库(如果存在)
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -U $DB_USER -d postgres -c "DROP DATABASE IF EXISTS copcon;"

# 3. 创建空数据库
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -U $DB_USER -d postgres -c "CREATE DATABASE copcon;"

# 4. 恢复数据
zcat $BACKUP_FILE | PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -U $DB_USER -d copcon

# 5. 验证数据完整性
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -U $DB_USER -d copcon -c "
  SELECT 'sessions' AS table, COUNT(*) FROM sessions;
  SELECT 'messages' AS table, COUNT(*) FROM messages;
"

# 6. 启动服务
sudo systemctl start copcon

# 7. 验证服务健康
sleep 5
curl http://localhost:8080/health
```

### RDS 时间点恢复

```bash
# 恢复到指定时间点
aws rds restore-db-instance-to-point-in-time \
  --source-db-instance-identifier copcon-db \
  --target-db-instance-identifier copcon-db-restored \
  --restore-time 2025-01-15T02:30:00Z

# 等待恢复完成
aws rds wait db-instance-available \
  --db-instance-identifier copcon-db-restored

# 修改 CopCon 配置指向恢复后的数据库
# DATABASE_HOST=copcon-db-restored.xxxxxx.region.rds.amazonaws.com

# 重启 CopCon
sudo systemctl restart copcon
```

### 云数据库恢复后的注意事项

恢复后数据库的连接地址通常不同(新实例名),需要:
1. 更新 CopCon 配置中的 `DATABASE_HOST`
2. 确保新数据库的安全组允许 CopCon 访问
3. 重启 CopCon 服务

## 灾难恢复计划

### RTO 和 RPO 定义

| 场景 | RTO | RPO | 方案 |
|------|-----|-----|------|
| 单实例故障 | 5 分钟 | 0 | Multi-AZ 自动切换 |
| 数据库损坏 | 1 小时 | 24 小时 | pg_dump 恢复 |
| 数据误删 | 2 小时 | < 1 小时 | PITR 恢复 |
| 全区域故障 | 4 小时 | 24 小时 | 跨区域备份恢复 |

### 故障响应流程

1. **发现**: 告警触发,值班人员确认
2. **评估**: 确定故障范围(进程/数据库/网络)
3. **决策**: 选择恢复方案(重启/恢复备份/切换区域)
4. **执行**: 按恢复流程操作
5. **验证**: 确认服务恢复正常
6. **记录**: 记录故障时间、原因、恢复耗时

### 跨区域灾难恢复

```
主区域: asia-east1
  └── CopCon 服务
  └── PostgreSQL (主)
  └── Qdrant (主)
  └── 定期备份 → S3/GCS (跨区域复制)

恢复区域: asia-northeast1
  └── 从备份恢复 PostgreSQL
  └── 从快照恢复 Qdrant
  └── 启动 CopCon 服务
  └── 切换 DNS
```

关键步骤:
1. S3/GCS 的跨区域复制必须启用
2. DNS 切换要自动化(Route53 health check / Cloud DNS)
3. 恢复区域的基础设施要提前部署好(或用 Terraform/IaC 快速创建)

## 下一步

- [生产检查清单](production-checklist.md)
- [故障排查](troubleshooting.md)
- [升级与迁移](upgrade.md)