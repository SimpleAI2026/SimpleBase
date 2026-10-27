---
title: 部署与故障处理
order: 2
---

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

`config.yaml` 是非密钥配置的权威来源（复制 `config.example.yaml`）。加载顺序：代码默认值 → YAML → `SIMPLEBASE_*` 环境变量覆盖。`SIMPLEBASE_CONFIG_PATH` 指向的文件缺失或非法 YAML 会使进程退出。

生产密钥（`SIMPLEBASE_S3_ACCESS_KEY` / `SIMPLEBASE_S3_SECRET_KEY` / `SIMPLEBASE_AUTH_APIKEY_SECRET` / `SIMPLEBASE_LLM_PROVIDER_*_API_KEY`）从 Secret 或 IAM 注入；YAML 里的非空密钥会被拒绝。

### instance

```yaml
instance:
  id: "simplebase-prod-1"
  writable: true  # 单写实例必须为 true
```

`id` 用于对象前缀与日志身份，不提供分布式锁语义。

### database

```yaml
database:
  engine: duckdb                 # 新建项目的默认单文件引擎：duckdb | sqlite
  cache_dir: "/var/lib/simplebase/cache"
  cache_max_bytes: 10737418240   # 10GiB 生产建议；代码/示例默认 1GiB
  cache_max_databases: 256
  idle_timeout: 5m
  max_open: 8
```

本地缓存可完全丢弃；丢失后从 S3 恢复。`cache_max_bytes` 与 `cache_max_databases` 触发 LRU 淘汰，活跃库不被淘汰。

### s3

```yaml
s3:
  endpoint: "https://s3.us-east-1.amazonaws.com"
  region: "us-east-1"
  bucket: "simplebase-prod"
  prefix: "simplebase"
  force_path_style: false
  kms_key_id: ""   # SSE-KMS CMK id；密钥材料走 IAM / SIMPLEBASE_S3_*
```

### system_database

```yaml
system_database:
  name: "simplebase-system"
  metrics_flush_interval: 2s
  log_flush_interval: 2s
  log_keep_days: 14
```

历史 `catalog.database_id` / `SIMPLEBASE_CATALOG_DATABASE_ID` 已删除，设置它们不再有任何效果。

### auth

```yaml
auth:
  api_key_hash_secret: ""  # 必填；生产用 SIMPLEBASE_AUTH_APIKEY_SECRET
```

`api_key_hash_secret` 用于 API key HMAC 签名。轮换需同时更新所有已签发 key。

### observability

```yaml
observability:
  log_level: info      # debug | info | warn | error
  log_format: json     # json | console
  log_output: stderr   # stderr | stdout
  metrics_path: /metrics
```

HTTP 优雅退出超时：`http.shutdown_timeout`（默认 30s）。

## HTTP 边界与浏览器直连

CORS、gzip 和限流默认关闭，安全响应头与 `X-Request-Id` 默认开启。跨域前端只放行明确来源；凭据模式禁止通配符：

```yaml
http:
  gzip: true
  trusted_proxies: ["10.0.0.0/8"]
  cors:
    enabled: true
    allowed_origins: ["https://app.example.com"]
    allowed_methods: [GET, POST, PUT, PATCH, DELETE, OPTIONS]
    allowed_headers: [Authorization, Content-Type, X-Request-Id]
    allow_credentials: false
limits:
  max_filter_scan_rows: 50000
  realtime_max_conns: 200
  rate_limit:
    enabled: true
    burst: 2
    exempt_sse: true
    rules:
      - match: "/v1/*"
        max: 100
        window: 1m
```

未配置可信代理时，`X-Forwarded-For` 不参与真实 IP 判定。限流返回 429 与 `Retry-After`；SSE 和文件下载不做 gzip。`SIMPLEBASE_*` 对应覆盖项见 `config.example.yaml`。

## Docker 部署

`Dockerfile` 已配置 distroless 非根镜像：

```bash
docker build -t simplebase .
docker run -d \
  --name simplebase \
  -p 8080:8080 \
  --env-file .env \
  -v /var/lib/simplebase/cache:/var/lib/simplebase/cache \
  simplebase
```

环境变量（密钥与可选覆盖；非密钥优先写 ConfigMap 中的 `config.yaml`）：

```text
SIMPLEBASE_CONFIG_PATH=/etc/simplebase/config.yaml
SIMPLEBASE_S3_ACCESS_KEY=...
SIMPLEBASE_S3_SECRET_KEY=...
SIMPLEBASE_AUTH_APIKEY_SECRET=...
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
        - name: config
          mountPath: /etc/simplebase
          readOnly: true
        env:
        - name: SIMPLEBASE_CONFIG_PATH
          value: /etc/simplebase/config.yaml
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
      - name: config
        configMap:
          name: simplebase-config
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

### 采集与告警建议

- Prometheus 以 15~60 秒间隔抓取 `/metrics`；该端点是实例级指标，**不含**项目请求趋势。项目级请求量、错误率、延迟看控制台[监控大盘](/docs/workbench/dashboard)或 `GET /v1/projects/:projectID/metrics/summary`。
- 建议至少对以下情况告警：`/health/ready` 连续失败超过 1 分钟；S3 错误持续上升；缓存目录接近 `cache_max_bytes`；大盘错误率持续偏高（见[监控大盘](/docs/workbench/dashboard)中的告警脚本示例）。
- `/metrics` 与健康检查端点只对内网开放，不要暴露到公网。

## 对象存储 CORS

控制台「对象存储」的文本与 Markdown 预览，会在浏览器里直接用预签名链接读取内容，因此需要 bucket 对控制台所在域名放行 `GET`。图片预览与「打开」不依赖 CORS。

以 AWS S3 为例（把 `AllowedOrigins` 换成你的控制台域名，不要使用 `*`）：

```json
[
  {
    "AllowedOrigins": ["https://console.example.com"],
    "AllowedMethods": ["GET", "HEAD"],
    "AllowedHeaders": ["*"],
    "MaxAgeSeconds": 3000
  }
]
```

其它 S3 兼容服务（MinIO、COS 等）的配置入口不同，规则相同：只放行 `GET` / `HEAD`，来源限定为控制台域名。未配置 CORS 时，文本预览会提示加载失败，「下载」会退回到新窗口打开临时链接，数据读写本身不受影响。本地开发模式使用本地文件存储，不涉及 CORS。

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

用户项目可通过 `/v1/projects/:projectID/backups` 创建、下载和恢复快照。恢复失败时保留原文件；进程在替换阶段中断后，下次打开项目文件会从 `.restore-old` 自愈。节点丢失本地缓存后，也可按 S3 上项目 manifest 指向的最新快照恢复。

### 场景 5：误启多实例

**症状**：第二个实例与现实例并行写同一 S3 前缀。

**处理**：
1. 确认仅一个实例运行（`replicas=1`）。
2. flock 锁仅防同机误配置；跨节点需部署层保证（如 K8s StatefulSet 唯一性）。
3. 若怀疑跨节点双写，立即停止后启动的实例，检查 catalog 状态。

## 备份策略

- **S3 在线数据**：每个项目的数据库以不可变快照加 manifest 存放在 `{project}/database/` 下，由项目级写租约保护；启用 bucket 版本控制作为对象级保护。
- **系统库**：实例级 DuckLake（`system_database.name`），与用户库隔离；用户库不使用 DuckLake。
- **独立备份 API**：`backup.enabled=true` 后，普通项目可通过 `/v1/projects/:p/backups` 创建、下载、删除和恢复快照，均需 `project:admin`。备份不含系统库，配置、保留策略和恢复验收见[备份与恢复](/docs/ops/backups)。
- **一致性代价**：快照在项目写租约内完成，大库会造成写排队。恢复先关闭项目句柄、校验 SHA-256/大小/引擎，再原子替换；失败回滚，S3 恢复后发布为新 manifest 版本。

## 事件触发器与运维边界

- 记录与对象事件使用有界进程内队列，队列满丢弃，重启不恢复未处理事件；需要持久工作流时应自行消费 `sys_go_func_invokes` 监控失败，不要把该能力当 exactly-once 消息系统。
- `/go`、调试试跑、cron 与触发器共享 `limits.max_concurrent_go_runs`，调高前应评估解释器 CPU/内存成本。
- `migrate status|plan` 只读检查最终态系统表，不执行变更；本项目无 down 迁移。
- 集合导入上限为 `limits.max_import_bytes`，导出是流式响应，应关闭这些路径的 gzip（默认中间件已跳过），CORS 需暴露 `Content-Disposition`。

## 安全基线

- S3 bucket 私有，禁止公开读。
- TLS 全链路（客户端→API、API→S3、API→LLM provider）。
- SSE-KMS 加密 S3 对象。
- 最小权限 IAM：API 仅需 S3 读写 + KMS Decrypt。
- LLM 密钥从 KMS 或加密配置加载，日志仅输出 provider/key ID 摘要。
- 审计日志默认不记录 SQL 参数与 LLM 正文（`Redact` 脱敏）。
- API key HMAC 签名，`auth.api_key_hash_secret` 32 字节随机，定期轮换。
