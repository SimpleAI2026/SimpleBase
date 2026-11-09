# internal 公共能力抽取计划（planv6.2）

> **状态：A1 / A2 / A3 / B1 / C 全部已落地**（见文末 §8 实施记录）。下文 §3–§7 保留为设计依据。
> 上一轮已完成 `internal/utils` 的建立与第一批抽取（见 §2），本文只规划**剩余部分**。
> 与 `plan/planv6.2/gocloudagent-module-refactor-plan.md` 无关，也不改变任何 HTTP 协议、存储格式或权限语义。

## 1. 目标与判定标准

目标：继续消除 `internal/` 内的跨包重复实现，让「同一件事只有一处实现」。

**准入标准（四条全满足才抽）**：

1. **零业务依赖**：落到 `internal/utils/<pkg>` 后只 import 标准库或已被广泛接受的第三方（`golang.org/x/...`、`google/uuid`）。不 import `catalog` / `auth` / `systemdb` / `api` / `database`。
2. **有重复事实**：至少两处独立实现，或一处实现已明显应该被多处复用。**「看起来通用」不算理由**。
3. **语义唯一**：一个包做一件事。若两处实现语义有差异（按字节 / 按 rune、返回 error / 返回零值、缓存错误 / 不缓存错误），必须先判定哪个正确，或**保持两份不合并**——不允许用参数开关糊在一起。
4. **调用方改动可验证**：每处替换都有现成测试覆盖，或能补出等价断言。

第 3 条是本次的硬约束。上一轮 `idgen` 建了又删，就是因为 5 处随机 ID 的签名各自绑定业务（有的返回 error、有的降级占位），强行统一会把差异推进业务层。

## 2. 已完成（上一轮，不在本文范围）

`internal/utils/` 现有四个包，均只依赖标准库：

| 包 | 收敛的重复 |
| --- | --- |
| `crontab` | 从 `gocloudagent` 迁出，`cronjob` + `api` 共用 |
| `strutil` | 截断 5 份 + CSV 切分 3 份 |
| `limit` | `Normalize` / `Clamp`，约 20 处 limit 归一 |
| `mask` | 敏感键正则 2 份（字节级相同）+ `MaskSecretValue` + `audit.Redact` |

基线：`go build ./internal/... ./cmd/...` 与 `go test ./...` 通过；`gocloudagent` 模块独立构建通过。

## 3. 剩余候选（已逐处核对代码）

### 3.1 A 批：纯函数，零并发语义，可直接落地

#### A1 `index SHA-256` → `internal/utils/hashx`

现状是 5 种写法混用，其中 4 处内联 `hex.EncodeToString(sum[:])`：

| 位置 | 形态 | 用途 |
| --- | --- | --- |
| `internal/api/sql_handler.go:513` `sqlSHA256` | 全量 hex | 审计只记摘要 |
| `internal/api/sandbox_handler.go:213` | 内联，全量 hex | 执行命令摘要 |
| `internal/database/ducklake/catalog_syncer.go:729` | 内联，全量 hex | 快照发布 |
| `internal/database/ducklake/catalog_syncer.go:1165` | 内联，全量 hex | 快照校验比对 |
| `internal/objectstore/blob.go:383` | 内联，**前 8 字节** | ETag |
| `internal/auth/session.go:167` `HashRefresh` | 全量 hex | refresh token 存储 |

接口：

```go
func Sum(data []byte) string              // 全量 hex
func SumString(s string) string           // 全量 hex（字符串入参）
func Short(data []byte, n int) string     // 前 n 字节 hex，n<=0 返回空
```

- `auth/service.go:68` 的 `HashKey` 是 HMAC（带 secret），**不合并**。
- `llmgateway/service.go:231` `credFingerprint` 是多字段 + `\x00` 拼接，**不合并**。
- `gocloudagent/trace.go:77` `hashSummary` 是独立模块，**不可引用**，保持原样。

风险：`objectstore/blob.go:383` 的 ETag 取前 8 字节，替换时不得变成全量（会改变对象元数据，虽然只在内存 fake store 中出现）。

#### A2 `tmp + rename` 原子写 → `internal/utils/atomicfile`

8 处同构，其中 1 处有缺陷：

| 位置 | 形态 | 备注 |
| --- | --- | --- |
| `internal/objectstore/localfilestore.go:98` | 固定 `.tmp` 名 | |
| `internal/objectstore/blob.go:232` | 固定 `.tmp` 名 | |
| `internal/objectstore/blob.go:405` | 固定 `.tmp` 名 | |
| `internal/app/identity.go:78` | 固定 `.tmp` 名 | |
| `internal/systemdb/locator.go:53` | 固定 `.tmp` 名 | **rename 失败时不清理 tmp**（其余 7 处都 `_ = os.Remove(tmp)`） |
| `internal/database/ducklake/local_state.go:75` | 固定 `.tmp` 名 | |
| `internal/database/ducklake/instance_format.go:243` | 固定 `.tmp` 名 | |
| `internal/api/storage_settings_handler.go:305` | `os.CreateTemp` + 先备份 `.bak` | 流式写，签名不同 |

接口：

```go
// Write 原子写入 path：同目录临时文件 → fsync? → rename；失败清理临时文件。
func Write(path string, data []byte, perm os.FileMode) error

// WriteEnsureDir 同 Write，但先 MkdirAll(filepath.Dir(path), 0o755)。
func WriteEnsureDir(path string, data []byte, perm os.FileMode) error

// WriteTemp 用 os.CreateTemp 创建同目录临时文件供流式写，close+rename 由调用方经 Commit 完成。
// 供 storage_settings_handler 这类「边写边算」的场景使用。
```

明确不做的事：

- **不加 `fsync`**。当前实现没有一个 sync 到磁盘，加 `fsync` 会改变启动路径的 IO 特性（`local_state` 每次水位更新都会 fsync）。如需崩溃一致性，作为独立议题评估。
- **不改临时文件名策略**为 `CreateTemp`。固定 `path+".tmp"` 存在并发写同一目标的覆盖风险，但改成 `CreateTemp` 会让权限从 0644 变成 0600（需额外 `Chmod`）、并让「残留 tmp 清理」逻辑失效（无法靠固定名删除）。这是行为变更，不在本次范围，单列为 §6 风险。
- `storage_settings_handler` 的 `.bak` 备份语义是它独有的，`atomicfile` 不承接备份，只承接 tmp+rename 那一段。

顺带修复：`locator.go` 的 tmp 清理缺口。

#### A3 SQL 标识符与字面量 → `internal/utils/sqlx`

这是本次**唯一带安全收益**的抽取：

| 函数 | 位置 | 行为 |
| --- | --- | --- |
| `quoteIdentifier` | `internal/api/data_handler.go:301` | `` `"` + name + `"` `` —— **不转义** |
| `quoteIdent` | `internal/database/ducklake/secret.go:61` | `"` + `ReplaceAll(name, `"`, `""`)` + `"` —— 正确 |
| `quoteSQLString` | `internal/database/ducklake/secret.go:58` | `'` + `ReplaceAll(s, `'`, `''`)` + `'` |
| `isSQLIdentByte` | `sqlguard/classifier-desens.go:134` 与 `api/system_redact.go:92` | **字节级相同** |
| `isSafeIdent` | `internal/database/ducklake/secret.go:70` | 首字符字母/下划线判定 |

两处 `quoteIdentifier` 同名不同义是真实隐患：`api/schema_rows.go:193` 对 `information_schema` 查出的列名调用它，而列名可由用户在 SQL 接口中创建（含 `"`），未经白名单直接拼进 SELECT。统一到转义版是纯增强。

接口：

```go
func Ident(name string) string    // "name"，内部双写 "
func Literal(s string) string     // 'value'，内部双写 '
func IsIdentByte(c byte) bool     // [A-Za-z0-9_]
func IsSafeIdent(name string) bool
```

约束：

- `api` 侧 20+ 处 `quoteIdentifier(...)` 调用改为 `sqlx.Ident(...)`，删除原函数。**不得**保留同名兼容包装（会重新引入两份）。
- `sqlguard` 与 `ducklake` 各自的私有别名同样删除，改调 `sqlx`。
- `sqlguard` 是 SQL 边界包，`sqlx` 只提供字面量构造，**不承担**校验职责：拒绝清单、只读检测仍只在 `sqlguard`。这条要写进 `sqlx` 包注释。
- 单测必须覆盖：名字含 `"`、含 `'`、含 NUL、空串。注意 `Ident("")` 会返回 `""`，调用方仍需自行拒绝空标识符。

### 3.2 B 批：并发语义，单独一次改动

#### B1 纯单飞 → `internal/utils/singleflight`

只有 `internal/database/lease/gate.go` 的 `Gate` 语义干净：per-key 单飞，`Do`（等待并复用结果）与 `TryDo`（不等待，立即返回 `ErrAcquiring`）。

`systemdb/query_cache.go` 的 `queryCache[T]` 是「TTL + 单飞 + 不缓存错误」，`catalog/cache.go` 的 `ttlCache[T]` 是「TTL + 容量 + 无单飞」，`auth/cache.go` 是「TTL + 负缓存 + 无单飞」。三者差异不是参数能消掉的：

- `queryCache` 的等待者复用 value/error，而 `Gate` 的等待者只复用 error；
- `catalog` 超容量时**全量清空**（不是 LRU），行为粗糙但是有意为之；
- `auth` 的负缓存是安全语义（防随机 key 打穿），不能当通用能力。

结论：**只抽 `Gate`，三个 TTL 缓存不动**。合并它们会把三种不同的失效策略塞进一个带开关的组件，违反准入标准第 3 条。

`Gate` 泛型化为 `singleflight.Group[T]`：

```go
type Group[T any] struct{ ... }
func New[T any]() *Group[T]
func (g *Group[T]) Do(key string, fn func() (T, error)) (T, error)     // 等待复用
func (g *Group[T]) TryDo(key string, fn func() (T, error)) (T, error)  // 不等待，在途返回 ErrInFlight
func (g *Group[T]) Forget(key string)
```

- `lease.Gate` 的 `Do` 语义有一处必须保留：等待者在首个调用**成功**时只拿到 `nil`，需自行复查快速路径（`ValidFor`/`Has`）。泛型化后 `lease` 侧仍保留这层包装，不能直接换成 value 复用。
- `ErrAcquiring` 是 `lease` 的业务哨兵（映射到 503 + Retry-After），`singleflight` 定义自己的 `ErrInFlight`，由 `lease` 用 `errors.Is` 转换。**不得**让 `singleflight` import `lease`。

风险：这是并发改动，必须带 `-race` 跑 `internal/database/lease`、`internal/database/registry` 及其上游 `internal/api`。`Gate` 当前有「首次调用 panic 时 `done` 未 close、等待者永久阻塞」的隐患——泛型化时用 `defer close(call.done)` 修掉，并补一条 panic 恢复用例。

### 3.3 C 批：反向简化（删代码，不建包）

这一批不新增公共包，但属于「简化代码」的正面收益，且能证明抽取不是唯一手段。

| 位置 | 现状 | 处理 |
| --- | --- | --- |
| `internal/api/context.go:97` | `func itoa(i int) string { return strconv.Itoa(i) }`，注释写「避免多处 import strconv」 | **删除别名**，7 次调用（全部在 `router.go`，6 行）改回 `strconv.Itoa`。`api` 已有 5 个文件 import strconv，这个别名没有避免任何事，只是多一层间接 |
| `internal/api/perfbench/loadgen.go:236` | `clampInt`，全仓唯一 clamp | 改用 `limit.Clamp(v, lo, hi)` 不合适（语义是 [lo,hi] 而非分页），**保留**；不为此新建 `clamp` 包 |
| `internal/config/env.go:392` | `envList` 手写 CSV 切分 | 改用 `strutil.SplitCSV`，但**保留 nil 语义**：未设置环境变量时仍返回 `nil`（现有测试 `config_more_test.go:135` 断言 nil） |
| `internal/config/env.go:188, 209` | 内联 `strings.Split` + 去空 | 可改 `strutil.SplitCSV`；`209` 是往已有 map 追加，需保留追加语义 |
| `internal/objectstore/client.go:402-421` | `derefStr` / `derefInt64` / `derefTime` | **不抽**。定义与全部调用点都在 `objectstore` 一个包内，不是跨包重复 |
| `internal/database/kv/set.go:360` | `dedupeStrings` | **不抽**。全仓唯一实现，无重复事实 |
| `internal/sbcli/commands.go:1280` | `strings.Split(p, ",")` | **不合并**。这是 JSON 解析失败后的兜底，不做 trim/去空，与 `SplitCSV` 语义不同 |
| `internal/database/serialize.go:104` / `sbcli/commands.go:304` / `systemdb/agents.go:697` | 三处「宽松 JSON 解析」 | **不合并**。返回值不同（`any` / `map{"ok":true}` / `error`），语义分歧大于共性 |
| 全仓 `strings.Contains(err.Error(), ...)` | 约 10 处错误字符串分类 | **不抽**。抽成工具会让脆弱的字符串匹配看起来像正规做法；正确方向是向哨兵错误收敛，属独立议题 |

## 4. 实施顺序与验收

每批独立可编译、可回退，不留半成品。

| 批次 | 内容 | 验收 |
| --- | --- | --- |
| A1 | `hashx` + 6 处调用替换 | `go test ./internal/api/... ./internal/objectstore/... ./internal/auth/... ./internal/database/ducklake/...`；断言 `Short(data, 8)` 与现有 ETag 逐字节相等 |
| A2 | `atomicfile` + 8 处替换 | 同上 + `./internal/app/... ./internal/systemdb/...`；补「rename 失败清理 tmp」用例，覆盖 `locator` 修复 |
| A3 | `sqlx` + 删除两套私有实现 | `go test ./internal/api/... ./internal/database/...`；新增含 `"` 的标识符用例，断言输出为 `"a""b"` |
| B1 | `singleflight` 泛型化 + `lease.Gate` 改用 | `go test -race ./internal/database/lease/... ./internal/database/registry/... ./internal/api/...`；补 panic 用例 |
| C | 删除 `itoa` 别名、`config` 复用 `SplitCSV` | `go test ./internal/api/... ./internal/config/...` |

每批完成后统一跑：

```bash
CGO_ENABLED=1 go build ./internal/... ./cmd/...
CGO_ENABLED=1 go test ./...
go vet ./internal/utils/... ./internal/database/... ./internal/api/...
(cd gocloudagent && GOWORK=off go build ./... && GOWORK=off go test ./...)
```

`gofmt`：`internal/` 现有 7 个生产文件未通过：

```
internal/api/agent_handler.go          internal/app/app.go
internal/api/gocloudagent_access.go      internal/database/ducklake/catalog_syncer.go
internal/api/error.go                  internal/llmgateway/service.go
internal/api/router.go
```

本次实际会改到其中两个：`catalog_syncer.go`（A1 的哈希替换）与 `router.go`（C 批删除 `itoa`，该文件有 7 次调用分布在 6 行）。按 `internal/AGENTS.md`「改动这些文件时顺手格式化」，这两个文件在对应批次里执行 `gofmt -w`，其余 5 个不动，**不做全仓重排**。

注意 `router.go` 的 `itoa` 全部出现在 `bodyLimit` 与访问日志中间件中，删除别名时逐个替换为 `strconv.Itoa`，不要顺手改动周边逻辑。

## 5. 不变量

以下行为在重构前后必须逐字相同，任何一条变化都视为失败：

1. 审计/日志仍只记 SHA-256 与元数据，不进 SQL 参数与 LLM 正文。
2. 系统库保护路径（`IsSystemDatabase`、`systemLeaseAdapter`）不受影响。
3. 分页默认值与上限：`Normalize` 超限回落默认值、`Clamp` 截到上限，两者不得互换。
4. `parsePermissions("")` 返回 `nil`、`config.envList` 未设置返回 `nil`、`SplitCSV("")` 返回非 nil 空切片。
5. `lease.Gate.Do` 的等待者仍只复用 error，成功时由调用方复查快速路径。
6. 数据库 schema、迁移、`init_db` 一律不碰。本次不执行任何 DDL。

## 6. 风险与不做的

1. **原子写并发**：固定 `path+".tmp"` 在并发写同一目标时会互相覆盖（`localfilestore.Put` 同一 key 并发、`local_state` 多写者）。本次**不修**，只是把现有行为收敛到一处，让将来统一修复只需改一个地方。修它需要单独评估权限位（0644 → 0600 + Chmod）与残留 tmp 清理策略。
2. **`quoteIdentifier` 转义**：改成转义版是纯增强，但会改变含 `"` 标识符的生成 SQL。现有 `collectionNamePattern` / `normalizeSchemaIdent` 白名单已排除这类名字，所以对合法输入输出不变；`schema_rows` 的列名来自 `information_schema`，可能含引号，这是本次要堵的口子。
3. **单飞泛型化的 panic 传播**：`Do` 内 `fn` panic 时必须先 close `done` 再继续 panic，否则等待者永久阻塞。这是本次必须新增的测试，不能只做 happy path。
4. **跨模块不可见**：`gocloudagent` 侧仍有 `agentcore.Truncate`、`tools.truncateText`、`cli_runner.limitOut`、`trace.hashSummary` 等副本。**不要**试图让它们 import `internal/utils`。若将来确需共用，按 `gosandbox` 模式新增零依赖小模块，由两侧 `require + replace`。
5. **不为对称而建包**：`clamp`、`deref*`、`dedupeStrings` 都因不满足准入标准被明确排除。评审时若有人提议加，先回到 §1 的四条。

## 7. 与既有计划的差异

- 上一轮排查报告曾建议「合并 6 份脱敏实现」为单一 `redact` 包。实际只做了 `mask`（键名判定 + 掩码），因为 `objectstore.redactor`（`scrubBetween` 按字节区间）、`api.system_redact`（列级白名单）、`gocloudagent.agentcore.RedactSecrets`（5 条正则）三者服务对象不同，合并会产出一个带大量开关的组件。**保持现状**，不再推进。
- 上一轮曾建议 `idgen` 统合 5 处随机 ID。因签名各自绑定业务而降级为「仅记录」，本文不再列入。

## 8. 实施记录

| 批次 | 结果 | 与计划的差异 |
| --- | --- | --- |
| A1 `hashx` | 已落地，6 处替换 | 无。`Short(cp, 8)` 保持 ETag 前 8 字节 |
| A2 `atomicfile` | 已落地，**7 处**替换 | 计划写 8 处，`storage_settings_handler` 因流式写 + `.bak` 备份语义独有而保留，未替换。`locator` 的 tmp 清理缺口随之修复，`atomicfile_test` 有 rename 失败清理用例。各调用点错误文案由「write X tmp / rename X」合并为「write X」，已核对无测试断言旧文案 |
| A3 `sqlx` | 已落地，删除 `api.quoteIdentifier`、`ducklake` 三个私有函数、两份 `isSQLIdentByte` | `IsSafeIdent` 沿用原 `unicode.IsLetter` 语义，与 ASCII 的 `IsIdentByte` 分开命名。新增 `TestSchemaRowsSelectEscapesQuotedColumn` 锁定列名含 `"` 时的转义 |
| B1 `singleflight` | 已落地，`lease.Gate` 改为薄封装 | `Gate.Do` 保持「等待者只复用 error」：泛型 `Group[struct{}]`，value 恒为零值。新增 panic 唤醒、`Forget` 不被旧 leader 误删用例；`lease`/`registry`/`utils` 均通过 `-race` |
| C | 已落地 | 删除 `api.itoa`（`router.go` 补 `strconv`，并 `gofmt`）；`config/env.go` 三处改用 `strutil.SplitCSV`，`envList` 保留「未设置返回 nil」。`context_test` 里对 `itoa` 的断言随别名删除（它在测 `strconv`） |

验收：`go build ./internal/... ./cmd/...`、`go vet ./internal/...`、`go test ./internal/... ./cmd/...` 全部通过；`-race` 覆盖 `utils` / `lease` / `objectstore` / `app`；`gocloudagent` 独立模块 `GOWORK=off` 构建与测试通过；`go list -deps ./internal/utils/...` 中除 utils 自身外无任何 SimpleBase 包。

遗留（沿用 §6，未处理）：固定 `path.tmp` 的并发覆盖风险；`gofmt` 仍有 5 个存量文件未通过（`agent_handler.go`、`gocloudagent_access.go`、`error.go`、`app/app.go`、`llmgateway/service.go`），本次未触碰它们。
