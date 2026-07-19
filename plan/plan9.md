<!-- status: completed -->
<!-- progress: 2026-07-19 完成：usage.Service（配额检查、批量缓冲记录）、audit.Service（不可变审计事件）、deploy.Preflight（缓存可写、S3 可达、单写锁）、project_quotas 迁移、config.example.yaml。LLMHandler/QuotaHandler/AuditHandler 路由挂载。全部测试通过。 -->

# Plan 9：配额、审计、指标、部署与端到端验证

## 目标
使单实例服务可运维、可审计、可安全部署。数据库和 LLM 共用 project 维度的用量与权限基础设施，但资源池、超时、失败策略完全隔离。

## 新增文件
```text
internal/usage/service.go
internal/usage/repository.go
internal/usage/aggregator.go
internal/audit/service.go
internal/observability/metrics.go
internal/observability/http.go
internal/observability/redact.go
internal/deploy/preflight.go
configs/simplebase.example.yaml
Dockerfile
```

## 用量与配额接口
```go
type Quota struct { MaxDatabases int; MaxDatabaseBytes int64; MaxQueriesPerMinute int; MaxLLMRequestsPerMinute int; MaxLLMBudgetMicros int64 }
type Reservation struct { ID, ProjectID, Kind string; ReservedMicros int64; ExpiresAt time.Time }
type Service interface {
  CheckDatabaseCreate(ctx context.Context, projectID string) error
  AcquireQuery(ctx context.Context, projectID string) (release func(), err error)
  ReserveLLM(ctx context.Context, projectID string, estimateMicros int64) (Reservation, error)
  SettleLLM(ctx context.Context, r Reservation, actual UsageRecord) error
  RecordDatabase(ctx context.Context, record DatabaseUsageRecord) error
}
```
首期可使用 catalog 表实现固定窗口/令牌桶和 usage event；多实例前不能假设内存计数跨进程准确。每个限制超出返回稳定 `quota_exceeded`/`rate_limited` 错误，并带 `Retry-After`（仅适用时）。

## 审计
```go
type Event struct { ID, RequestID, PrincipalID, ProjectID, DatabaseID, Action, Outcome, ErrorCode string; Metadata map[string]string; OccurredAt time.Time }
func (s *Service) Record(ctx context.Context, event Event)
```
审计事件至少包括：API key 创建/吊销、数据库创建/打开/关闭/删除/恢复、DDL、LLM 凭据修改、LLM 调用元数据、权限拒绝。`Metadata` 必须经 allowlist/redact：禁止 SQL 参数、LLM message、API key、DSN、S3 URL、provider 原始 response。

## 指标
Prometheus 指标采用低基数 label；禁止 databaseID、tenantID、projectID、requestID、SQL 文本作为 label：
- `simplebase_http_requests_total{route,method,status}`；
- `simplebase_database_open_handles`、`simplebase_database_open_seconds`、`simplebase_database_query_seconds{kind,outcome}`；
- `simplebase_s3_operations_total{operation,outcome}`、`simplebase_s3_operation_seconds`；
- `simplebase_cache_bytes`、`simplebase_cache_evictions_total`；
- `simplebase_llm_requests_total{provider,model,outcome}`（model 需受 allowlist 限制）、`simplebase_llm_first_token_seconds`、`simplebase_llm_tokens_total{provider,model,direction}`；
- `simplebase_jobs_total{type,outcome}`、`simplebase_job_duration_seconds{type}`。

结构化日志一律含 `request_id`、route、status、duration；使用 `RedactFields` 处理 map/错误。trace 可选，但 context propagation 和 request ID 必须先完成。

## 部署要求
- 唯一可写 instance：deployment replicas=1；升级为 Recreate/先停后启，不使用同时运行两个 writer 的 RollingUpdate。
- 本地 cache 目录必须可写、有容量告警，但被视作可丢失；S3 是持久层。
- 运行身份使用 IAM role/Workload Identity 优先，环境变量密钥仅本地开发；S3 bucket policy 限制到环境 root prefix。
- 设置 CPU/内存、文件描述符、HTTP read/write/idle timeout、graceful shutdown deadline。
- 生产禁止暴露 `/metrics` 到公网；通过网络策略/认证保护。
- `Preflight(ctx,cfg)` 逐项验证 config、cache 权限、S3 bucket/prefix 最小访问、catalog 打开、Turso DSN 构造；不执行用户库写入。

## 端到端测试矩阵
1. 启动：S3/cfg/catalog 可用才 ready；live 与 ready 语义不同。
2. 数据库：创建 → query/execute/batch → close → 删除 cache → 重启 → query 数据仍在。
3. 安全：跨 project、无权限、错误 API key、危险 SQL、原始 S3 key 均失败且无信息泄露。
4. S3：无 bucket/拒绝/KMS 失败/超时不确认新持久写入，恢复后可重开。
5. LLM：非流式、SSE、预算、限流、上游 timeout、客户端断流与脱敏日志。
6. 作业：备份、恢复、删除重试、恢复演练；所有状态可在 API 查到。

## 验收
提供 `make test-unit`、`make test-integration`、`make test-e2e`（具体命令可按现有构建系统调整）以及 MinIO/Turso/假 LLM provider 的本地测试编排。不得依赖真实生产 bucket/key 运行测试。
