<!-- status: completed -->
<!-- progress: 2026-07-19 全部完成。删除顺序步骤 1-6 均已执行：旧链路移除、go mod tidy 清理、README/internal README/部署文档/迁移指南更新。新代码零依赖旧链路，go build/go test 通过。 -->

# Plan 10：迁移、切换、旧链路退役与后续扩展边界

## 目标
将现有自研 HA SQLite/Raft/VFS 和旧 S3 HTTP 接口安全迁移到 Turso + S3 新链路；仅在契约、恢复、压测与回退验证完成后删除旧实现。本计划是最后执行的清理阶段，不可提前执行。

## 现有代码清单与决策
| 路径 | 现状 | 新架构处理 |
| --- | --- | --- |
| `cmd/main.go` | Echo + 旧 `server` 启动 | 改为兼容调用 `cmd/simplebased`，稳定后删除 |
| `server/server.go`、`server/storage.go` | S3 upload/append/select HTTP API | 下线/迁移为独立对象存储产品，不能作为数据库 API |
| `internal/s3/s3_append.go`、`s3_select.go` | 自研 AppendObject/SelectObject | 不用于 Turso 数据库；无独立需求则删除 |
| `internal/database/db/*` | HA SQLite DB/VFS 管理 | 提取契约测试后删除 |
| `internal/database/driver/*` | 自研 SQL driver | 用 `tursogo` 替代，删除 |
| `internal/database/node/*` | Raft、leader、snapshot | 首期不需要，删除 |
| `internal/database/db/walfs/*` | WAL/VFS 复制 | 首期不需要，删除 |
| `internal/database/gorm_driver/*` | 旧 GORM driver | 若模型仍依赖，新增基于 `database/sql`/Turso 的 adapter；否则删除 |
| `internal/database/proto/*`、旧 cmd | 旧 gRPC 协议和 CLI | 客户端迁移后删除 |
| `/litellm` | 多 provider 客户端 | 保留，不挪入旧链路 |

## 迁移前门槛
满足下列条件才可导入第一批用户库：
1. Plan 1–9 的单元、集成、端到端测试通过；空 cache 从 S3 恢复已演练。
2. 有独立开发/预发布 S3 bucket、KMS、最小 IAM 与成本告警。
3. 新 API 具备认证、project 隔离、审计、限流、备份/恢复任务。
4. 数据库 schema、SQLite feature、数据类型、触发器/外键和自定义 extension 有兼容清单。
5. 写入与持久化确认语义经 Turso 官方文档和实际故障测试确认。

## 单库迁移工作流
```text
discover → precheck → snapshot → import → validate → shadow-read → freeze-writes
→ final-snapshot/import → validate → route-switch → observe → retire-old
```

### 1. Discover / Precheck
实现 `internal/migration/inventory.go`：读取旧库元数据、文件大小、schema dump、`PRAGMA user_version`、`foreign_key_check`、`integrity_check`、对象数量和 checksum。将不可兼容项列入 report，禁止自动忽略。

### 2. Snapshot / Import
使用旧系统支持的一致性快照导出（或停写窗口），通过官方 Turso 导入/恢复方式生成新 database ID 与 S3 prefix。禁止边运行边复制 SQLite/WAL 文件。记录 `MigrationRecord{sourceID,targetID,sourceChecksum,targetChecksum,status}`。

### 3. Validate
实现 `internal/migration/validate.go`：
```go
type Validator interface {
  Schema(ctx context.Context, source, target *sql.DB) error
  Integrity(ctx context.Context, target *sql.DB) error
  Counts(ctx context.Context, source, target *sql.DB) error
  SampleRows(ctx context.Context, source, target *sql.DB, seed int64, limit int) error
}
```
先做 schema diff/完整性/每表 count，再固定 seed 抽样 hash。对关键库可全表分块 checksum；不接受“仅能打开”作为迁移成功。

### 4. Shadow Read
应用读请求在不影响响应的情况下同时访问新库，比较受控、幂等的 query 结果与错误码；绝不 shadow write。比较任务限流、脱敏并可随时关闭。发现差异时不切换。

### 5. Freeze 与切换
维护窗口：阻止旧库写入 → 执行最后一致快照/导入 → 全量校验 → 更新应用配置/API alias 指向 new database ID → 连续观测。新旧库不存在双写。切换操作必须产生审计事件和可回退的 alias 版本。

### 6. 回退与退役
在观察期内，若错误预算超阈值且旧库未被写入，则将 alias 切回旧库；切换后不应继续向新库写入以免产生不可合并分叉。观察期结束后旧库只读、备份、按保留期清理。

## 兼容层策略
- 对旧 `/api/v1/createdb`、`execute`、`query`，新增只转发到 Plan 5/6 service 的 adapter；必须注入 project，不能保留无认证全局操作。
- response 可临时映射旧字段，但新增字段只在 v1 新 API 输出；通过 `Deprecation`/`Sunset` header 公告期限。
- 不为兼容旧客户端保留原始 S3 key、AppendObject 或旧 driver 的写能力。

## 删除顺序
1. 移除生产路由到旧 writer，确认所有调用路径进入新 Registry。
2. 删除旧命令、gRPC service、tests/http_* 旧接口测试，替换为新 API 契约测试。
3. 删除 `internal/database/node`、`walfs`、旧 driver、旧 VFS 和无用 proto。
4. 删除旧 Append/Select storage handlers 与对应 S3 实现/依赖。
5. `go mod tidy` 后移除 Raft、旧 SQLite/VFS、gRPC 等未使用依赖；仅在代码删除后修改 `go.mod/go.sum`。
6. 更新 README、部署文档、迁移指南、架构图和 runbook。

每一步独立 PR/提交并执行 `go test ./...`、迁移回归和 S3 恢复演练；任何一步失败均停止后续删除。

## 后续扩展边界（不在本次实现）
- 多 API 实例、强一致 catalog、distributed writer ownership、lease/epoch/fencing；实现前严禁将内存 Registry 误用于多实例。
- 自动只读 replica、跨区域/边缘读、全球调度。
- Serverless 函数运行时：只能通过受限 service token、显式 project context 和限额访问 DB/LLM。
- Auth、Storage、Realtime、Web Studio、向量检索等 Supabase 化能力。

## 最终验收
- 不存在两条可写数据库路径；扫描仓库确认业务新代码未 import 旧 driver/node/walfs。
- 新服务可从空 cache + S3 恢复 catalog 与用户库；演练报告可追溯。
- 租户隔离、SQL/LLM 限制、审计与指标通过安全评审。
- 旧代码/依赖已删除或有明确保留理由；README 只描述新架构与迁移后的 API。
