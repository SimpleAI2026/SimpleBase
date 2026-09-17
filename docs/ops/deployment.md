# SimpleBase 部署文档与 Runbook

## 部署模型

SimpleBase 首期为**单写实例**部署。所有写请求经同一进程的 Database Registry 路由，保证同一 logical database 只有唯一 writer。

### 硬性约束

| 约束 | 要求 | 原因 |
| --- | --- | --- |
| 副本数 | 固定 `replicas=1` | API 路由只能保证单进程内单写；多实例需外部分布式锁 |
| 滚动升级 | 先停旧实例，再启新实例 | 禁止新旧实例并行写同一 S3 前缀 |
| S3 bucket | 私有 + TLS + SSE-KMS + 版本控制 | 持久层安全基线 |
| Catalog 库 | 独立于用户库，专属 S3 前缀 | 防止用户 SQL 影响平台元数据 |
| 凭据 | 环境变量或 IAM 角色注入 | 禁止写入配置文件或镜像 |

## 配置

参考 `config.example.yaml`。关键字段：

### instance

```yaml
instance:
  id: "simplebase-prod-1"
  writable: true  # 单写实例必须为 true
```

`id` 用于启动冲突检测（防误配置），不提供分布式锁语义。

### database

```yaml
database:
  engine: ducklake
  cache_dir: "/var/lib/simplebase/cache"
  cache_max_bytes: 10737418240   # 10GiB
  cache_max_databases: 256
  idle_timeout: 5m
```

本地缓存可完全丢弃；丢失后从 S3 恢复。`cache_max_bytes` 与 `cache_max_databases` 触发 LRU 淘汰，活跃库不被淘汰。

### s3

```yaml
s3:
  endpoint: "https://s3.us-east-1.amazonaws.com"
  region: "us-east-1"
  bucket: "simplebase-prod"
  prefix: "simplebase/"
  access_key_id: ""      # 环境变量或 IAM 角色
  secret_access_key: ""
  force_path_style: false
```

### system_database

```yaml
system_database:
  name: "simplebase-system"   # SIMPLEBASE_SYSTEM_DB_NAME
  hide_from_list: true
```

历史 `catalog.database_id` / `SIMPLEBASE_CATALOG_DATABASE_ID` 已删除，设置它们不再有任何效果。

### auth

```yaml
auth:
  server_secret: ""      # 32 字节随机密钥，环境变量注入
  default_plan: "free"
```

`server_secret` 用于 API key HMAC 签名。轮换需同时更新所有已签发 key。

## Docker 部署

`Dockerfile` 已配置 distroless 非根镜像：

```bash
docker build -t simplebased .
docker run -d \
  --name simplebase \
  -p 8080:8080 \
  --env-file .env \
  -v /var/lib/simplebase/cache:/var/lib/simplebase/cache \
  simplebased
```

环境变量（`SIMPLEBASE_` 前缀覆盖同名配置）：

```text
SIMPLEBASE_S3_ACCESS_KEY=...
SIMPLEBASE_S3_SECRET_KEY=...
SIMPLEBASE_AUTH_APIKEY_SECRET=...
SIMPLEBASE_SYSTEM_DB_NAME=simplebase-system
```

## Kubernetes 部署

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: simplebase
spec:
  replicas: 1                    # 必须为 1
  updateStrategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1          # 先停旧 Pod
      maxSurge: 0                # 禁止新旧并行
  template:
    spec:
      containers:
      - name: simplebase
        image: simplebase:latest
        ports:
        - containerPort: 8080
        envFrom:
        - secretRef:
            name: simplebase-secrets
        volumeMounts:
        - name: cache
          mountPath: /var/lib/simplebase/cache
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8080
          initialDelaySeconds: 5
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8080
          initialDelaySeconds: 10
      volumes:
      - name: cache
        emptyDir:
          sizeLimit: 10Gi
```

> 缓存卷可丢失。Pod 重启后从 S3 恢复。使用 `emptyDir` 而非持久卷，强调缓存语义。

## 启动检查

`internal/deploy` 已删除。启动时由 `cache.NewManager` 校验缓存目录，`/health/ready` 检查系统库与 S3。单写仍由部署层保证（`replicas=1`）。

## 健康检查

| 端点 | 含义 | 失败行为 |
| --- | --- | --- |
| `GET /health/live` | 进程存活 | 5xx → 重启 Pod |
| `GET /health/ready` | Catalog 可用 + S3 可达 | 5xx → 流量摘除，不重启 |

`ready` 失败时 API 返回 503，禁止写入。恢复后自动转 200。

## 指标

`GET /metrics`（路径可配）暴露 Prometheus 指标：

- 数据库：打开数量、冷启动延迟、查询延迟、事务失败、S3 延迟/错误、缓存命中、容量。
- LLM：provider/model 请求量、首 token 延迟、总延迟、token、成本、限流、重试、流中断。

## Runbook

### 场景 1：S3 故障

**症状**：`/health/ready` 返回 503；写请求失败。

**处理**：
1. 确认 S3 状态（AWS 控制台或 `aws s3 ls`）。
2. S3 恢复后 `/health/ready` 自动转 200。
3. 禁止在 S3 故障期间强制写入（会掩盖持久层不可用）。
4. 恢复后对关键库执行 `PRAGMA integrity_check`（通过 SQL API）。

### 场景 2：Pod 重启 / 缓存丢失

**症状**：Pod 重启，本地缓存清空。

**处理**：
1. 自动恢复：Registry 从 catalog 重建，按需从 S3 恢复数据库。
2. 首次访问某库时冷启动延迟较高（S3 下载 + 完整性检查）。
3. 验证：`GET /health/ready` 为 200 后执行业务查询。
4. 无需手动干预；缓存为可重建状态。

### 场景 3：滚动升级

**约束**：禁止新旧实例并行。

**步骤**：
1. 旧实例收到 SIGTERM → `Shutdown`（30s 超时）停止接收新请求、等待在途请求、关闭数据库连接。
2. 旧实例退出后，新实例启动 → `ready` 为 200。
3. 流量切换到新实例。

> 若使用 K8s `RollingUpdate`，必须设 `maxSurge: 0` + `maxUnavailable: 1`。

### 场景 4：数据库恢复

备份/恢复 HTTP 路由（`/backups`、`/restore`）与 jobs worker 已退役。节点丢失本地缓存后，从 S3 上的 DuckLake catalog / DATA_PATH 重建。

### 场景 5：误启多实例

**症状**：第二个实例与现实例并行写同一 S3 前缀。

**处理**：
1. 确认仅一个实例运行（`replicas=1`）。
2. flock 锁仅防同机误配置；跨节点需部署层保证（如 K8s StatefulSet 唯一性）。
3. 若怀疑跨节点双写，立即停止后启动的实例，检查 catalog 状态。

## 备份策略

- **S3 在线数据**：DuckLake catalog 与 Parquet DATA_PATH 存在对象存储；启用 bucket 版本控制作为对象级保护。
- **系统库**：实例级 DuckLake（`system_database.name`），与用户库隔离。
- **独立 backups/restore API**：已移除（此前仅 501）。

## 安全基线

- S3 bucket 私有，禁止公开读。
- TLS 全链路（客户端→API、API→S3、API→LLM provider）。
- SSE-KMS 加密 S3 对象。
- 最小权限 IAM：API 仅需 S3 读写 + KMS Decrypt。
- LLM 密钥从 KMS 或加密配置加载，日志仅输出 provider/key ID 摘要。
- 审计日志默认不记录 SQL 参数与 LLM 正文（`Redact` 脱敏）。
- API key HMAC 签名，`server_secret` 32 字节随机，定期轮换。
