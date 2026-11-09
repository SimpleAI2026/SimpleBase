# internal/utils

跨包复用的通用能力。**这里只放与业务无关的纯函数**——不认识的类型进来就说明放错地方。

## 入库标准

一条代码要落到 `internal/utils/<pkg>`，必须同时满足：

1. **零业务依赖**：只 import 标准库（下表所有包目前都是）。不得 import `catalog`、`auth`、`systemdb`、`api`、`database` 等任何根模块业务包，否则会把依赖方向拉平、制造反向 import。
2. **有复用事实**：至少两处调用，或已经存在多份同义实现。只为「看起来通用」而抽一个函数不算。
3. **语义单一且可命名**：一个包做一件事。语义有分歧（例如 `path` 与 `filepath`、按字节与按 rune）必须先统一，不能靠参数开关糊在一起。

## 现有包

| 包 | 职责 | 替代的历史重复实现 |
| --- | --- | --- |
| `crontab` | 受限 5 字段 cron 解析与下次触发计算（UTC，分钟粒度） | 从 `gocloudagent` 迁出；`cronjob`、`api` 共用 |
| `strutil` | 字符串截断（字节 / UTF-8 安全 / 带省略号）与 CSV 列表切分 | `cronjob.truncate`、`api.truncateMessage`、`agentbridge.limitOut`、`sandbox.truncateUTF8`、`agentbridge` 定时任务错误裁剪、`api.splitNonEmpty`、`auth.parsePermissions` |
| `limit` | 分页参数归一化（`Normalize` 回落默认值 / `Clamp` 截到上限） | 约 20 处 `if limit <= 0 \|\| limit > N` |
| `mask` | 敏感字段名判定与密钥掩码 | `sbcli.sensitiveKey`、`agentbridge.sensitiveKeyPattern`（正则字面量曾完全重复）、`systemdb.MaskSecretValue` |
| `hashx` | 无密钥 SHA-256 的 hex 封装（`Sum` / `SumString`） | `api.sqlSHA256`、`sandbox_handler` 内联、`catalog_syncer` 两处内联、`auth.HashRefresh`、`objectstore` 内存 ETag（取前 8 字节） |
| `atomicfile` | 同目录 `path.tmp` + rename 原子写，失败清理临时文件；不 fsync | `localfilestore`、`blob`（2 处）、`app/identity`、`systemdb/locator`、`ducklake/local_state`、`ducklake/instance_format`；顺带修复 `locator` rename 失败不清理 tmp |
| `sqlval` | database/sql 参数值归一化（`BoolInt` / `NullTime` / `NullTimeValue` / `NullString`） | `auth.boolToInt`、`collection.boolInt`、`systemdb.triggerBoolInt`、`auth.nullTime` 与 `catalog.nullTime`（逐字相同）、`systemdb.nullTimeValue` / `nullString` |
| `sqlx`（位于 `godatabase/sqlx`） | SQL 标识符 / 字面量转义（`Ident` / `Literal`）与标识符字符判定 | `api.quoteIdentifier`（**原先不转义 `"`**）、`ducklake.quoteIdent` / `quoteSQLString` / `isSafeIdent`、`sqlguard` 与 `api` 两份字节级相同的 `isSQLIdentByte` |
| `singleflight`（位于 `godatabase/internal/singleflight`） | 泛型按 key 单飞，`Do`（等待复用）/`TryDo`（在途返回 `ErrInFlight`）/`Forget`；leader panic 时唤醒等待者 | `lease.Gate` 的内部实现（`Gate` 保留为租约领域薄封装，负责把 `ErrInFlight` 译成 `ErrAcquiring`） |

## 不要放这里

- `testutil/`：测试基础设施（fake LLM server 等），带 httptest 依赖，不属于生产工具。留在 `internal/testutil`。
- 绑定具体驱动/协议的能力：`database/serialize.go`（DuckDB 类型）、`api/system_redact.go`（业务列名白名单）、`web`（`go:embed` 装配）。
- 适配层本体：`agentbridge/*` 围绕 `auth.Principal` / `systemdb.*` 双向映射（含 agent 定时任务执行），抽走等于把业务带进来。
- 终端尺寸归一化：`sandbox.ClampPTYDim` 属于沙盒领域规则，e2b 协议与 sshd 会话共用，不进本目录。
- 依赖生命周期的设施：`database/lease`、`registry`、`cache`。
- 带 TTL / 容量 / 负缓存的缓存：`systemdb.queryCache`、`catalog.ttlCache`、`auth.apiKeyCache` 失效策略各不相同，`singleflight` 只做单飞、不缓存。
- 带密钥或多字段的摘要：`auth.HashKey`（HMAC）、`llmgateway.credFingerprint`（多字段拼接）不进 `hashx`。

## 跨模块限制

`gocloudagent/`、`gosandbox/`、`gofunction/` 是嵌套独立模块，**不能 import 根模块 `internal/...`**。因此本目录的能力对它们不可见，下面的副本是有意保留的，不要试图「统一」到 `internal/utils`：

| 能力 | gocloudagent 侧位置 | 说明 |
| --- | --- | --- |
| rune 截断 | `gocloudagent/agentcore/redact.go` `Truncate` | 带 `\n…[truncated]` 标记，按 rune 而非字节截断 |
| 字节截断 | `gocloudagent/tools/tools.go` `truncateText` | 跨模块无法共享 |
| CLI 输出上限 | `gocloudagent/cli_runner.go` `limitOut` | 与 `agentbridge/cli.go` 字节级相同，受模块边界阻断 |

若将来这些副本确实成为负担，正确做法是像 `gosandbox/` 那样新增一个独立小模块（零第三方依赖），由两侧 `require + replace`，而不是让 `gocloudagent` 反向依赖根模块。

## 新增包时

1. 目录名即包名，小写单词，不猜缩写。
2. 包注释写清职责与**边界**（什么情况该用哪个函数）。
3. 带单元测试，覆盖边界值（空串、负数、恰好等于上限、多字节字符）。
4. 改动调用点时保持原有 nil / 空切片 / JSON 输出语义不变。
