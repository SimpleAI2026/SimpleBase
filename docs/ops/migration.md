# SimpleBase 迁移指南

> **Deprecated（历史存档）**：本文描述的是 LessDB/ha-sqlite → Turso/libSQL + S3 的第一次迁移。Turso/libSQL 引擎已退役；当前运行时是 DuckLake-only + S3。保留本文仅供归档查阅，不要当作现行部署指南。

从旧版本（LessDB / ha-sqlite，基于自研 Raft/VFS/SQLite）迁移到 Turso/libSQL + S3 新链路。

## 迁移前门槛

满足以下条件才可导入第一批用户库：

1. Plan 1–9 单元、集成、端到端测试通过；空 cache 从 S3 恢复已演练。
2. 有独立开发/预发布 S3 bucket、KMS、最小 IAM 与成本告警。
3. 新 API 具备认证、project 隔离、审计、限流、备份/恢复任务。
4. 数据库 schema、SQLite feature、数据类型、触发器/外键兼容清单已确认。
5. 写入与持久化确认语义经 Turso 官方文档和实际故障测试确认。

## 兼容性说明

### 已删除的旧能力（不再支持）

| 旧能力 | 处理 |
| --- | --- |
| 自研 AppendObject / SelectObject | 删除；不作为数据库 API |
| 旧 `/api/v1/createdb`、`execute`、`query` | 不保留兼容层；新链路使用 `/v1/projects/:p/...` |
| Raft / WAL 复制 / 自研 VFS | 删除；单写实例 + S3 持久层替代 |
| 旧 gRPC 协议与 CLI | 删除；客户端迁移到新 HTTP API |
| GORM driver | 删除；模型通过 `database/sql` + Turso adapter 接入 |
| 无认证全局操作 | 禁止；所有操作必须注入 project + 认证 |
| 原始 S3 key 写能力 | 不保留 |

### 保留的领域模型

现有业务模型保留，通过 repository / connection factory 接入新数据库层。迁移不要求重写模型。

## 单库迁移工作流

```text
discover → precheck → snapshot → import → validate → shadow-read → freeze-writes
→ final-snapshot/import → validate → route-switch → observe → retire-old
```

### 1. Discover / Precheck

读取旧库元数据、文件大小、schema dump、`PRAGMA user_version`、`foreign_key_check`、`integrity_check`、对象数量与 checksum。将不可兼容项列入 report，禁止自动忽略。

### 2. Snapshot / Import

使用旧系统支持的一致性快照导出（或停写窗口），通过 Turso 官方导入/恢复方式生成新 database ID 与 S3 prefix。**禁止边运行边复制 SQLite/WAL 文件**。

记录迁移记录：

```text
MigrationRecord{
  sourceID, targetID,
  sourceChecksum, targetChecksum,
  status
}
```

### 3. Validate

校验顺序：
1. **Schema diff**：源库与目标库 schema 一致。
2. **Integrity**：目标库 `PRAGMA integrity_check` 通过。
3. **Counts**：每表行数一致。
4. **SampleRows**：固定 seed 抽样 hash 一致。

对关键库可全表分块 checksum。**不接受“仅能打开”作为迁移成功。**

### 4. Shadow Read

应用读请求在不影响响应的情况下同时访问新库，比较受控、幂等的 query 结果与错误码。**绝不 shadow write。**

- 比较任务限流、脱敏、可随时关闭。
- 发现差异时不切换。

### 5. Freeze 与切换

维护窗口：
1. 阻止旧库写入。
2. 执行最后一致快照 / 导入。
3. 全量校验。
4. 更新应用配置 / API alias 指向 new database ID。
5. 连续观测。

**新旧库不存在双写。** 切换操作必须产生审计事件和可回退的 alias 版本。

### 6. 回退与退役

- 观察期内若错误预算超阈值且旧库未被写入，将 alias 切回旧库。
- 切换后不应继续向新库写入（避免不可合并分叉）。
- 观察期结束后旧库只读、备份、按保留期清理。

## API 映射

| 旧 API | 新 API | 说明 |
| --- | --- | --- |
| `POST /api/v1/createdb` | `POST /v1/projects/:p/databases` | 需认证 + project 上下文 |
| `POST /api/v1/{key}/execute` | `POST /v1/projects/:p/databases/:id/execute` | 参数化 SQL，权限 `database:write` |
| `POST /api/v1/{key}/query` | `POST /v1/projects/:p/databases/:id/query` | 权限 `database:read` |
| `GET /api/v1/{key}/tables` | 通过 SQL API `SELECT name FROM sqlite_master WHERE type='table'` | 无独立端点 |
| `GET /api/v1/{key}/tables/{t}/rows` | 通过 SQL API `SELECT * FROM {t} LIMIT ? OFFSET ?` | 无独立端点 |
| `POST /api/v1/{key}/executelog` | 无对应 | 新链路同步执行，无异步日志查询 |

### 客户端迁移要点

1. **认证**：旧 API 用 readkey/writekey 路径参数；新 API 用 `Authorization: Bearer <api-key>` + project 路径。
2. **project 绑定**：所有请求必须显式携带 project ID。
3. **SQL 参数化**：新 API 强制参数化，禁止拼接。
4. **响应格式**：新 API 返回统一 JSON 协议（见 `internal/api/sql_types.go`）。
5. **错误码**：新 API 使用标准 HTTP 状态码 + JSON error body（见 `internal/api/error.go`）。

## 回退演练

切换前必须演练回退路径：
1. 模拟新库故障 → alias 切回旧库。
2. 验证旧库在冻结期未被写入（checksum 不变）。
3. 验证应用能在 5 分钟内切换 alias。
4. 记录演练报告，含切换时间、校验结果、回退步骤。

## 迁移后验收

- 扫描仓库确认业务新代码未 import 旧 driver/node/walfs（已通过）。
- 新服务可从空 cache + S3 恢复 catalog 与用户库；演练报告可追溯。
- 租户隔离、SQL/LLM 限制、审计与指标通过安全评审。
- 旧代码/依赖已删除；仓库只描述新架构与迁移后的 API。
