# Key-Value 数据服务：项目级 KV，底层 DuckLake

> **仓库**：SimpleBase（https://github.com/linkxzhou/SimpleBase）
> **状态**：方案修订（2026-09-27）。存储层 `internal/database/kv` 沿用；产品面按本文改。
> **对外协议**：见 `docs/database/kv.md`（与本文 §5 一致，文档给调用方，本文给实现）。
> **关联**：`internal/AGENTS.md`、`internal/database/README.md`。

---

## 0. 目标

每个项目创建后自带一份 Key-Value，控制台和 HTTP 都按**项目**使用，不选择数据库。

对外只有一个写入口：

`POST /v1/projects/:projectID/kv`

请求体是 Redis 语义的 JSON，两种形态：

| `type` | 体 | 用途 |
|---|---|---|
| `cmd` | `argvs`: 命令参数数组，首元素是命令名 | Redis 命令（读、改、删、扫描、过期、自增…） |
| `String` / `Hash` / `List` / `Set` / `ZSet` | `args`: 该类型的数据 | 按类型添加或写入一份 KV |

底层仍是该项目专属的一份 DuckLake catalog 里的 `kv` schema（`kv.keys` + 五张数据表）。这份 catalog **不出现在数据库列表**，用户 SQL 库与 KV 无关。

仓库层（`internal/database/kv` 的六类 SQL、类型守卫、TTL、空结构删除）不改语义。改的是：KV 挂在哪、HTTP 长什么样、控制台还要不要选库。

### 本版删除

下列设计作废，文档、路由、控制台不得再沿用：

- 控制台 Key-Value 页的数据库下拉，以及「先建数据库再回来管 KV」的空态。
- 数据库「查看数据」里的 Key-Value 页签（`DataTabs` 不再包一层 KV）。
- `/databases/:databaseID/kv/...` 按资源拆开的 REST（`/strings/:key`、`/hashes/...`、`_mget` 等）。
- 每个用户库 `EnsureSchema`，以及建项目时创建名为 `default` 的用户库来装 KV、响应里的 `default_database`。
- 把「key 不存在」映射成 HTTP 404。Redis 的读命令对缺失 key 返回 nil / 0 / `none`，不是错误。

---

## 1. 存储仍参考 redka，SQL 仍针对 DuckLake

redka 把 Redis 语义落在关系表上（key 元数据 + string/list/set/hash/zset）。本方案只借鉴模型与命令语义，**不 import** `thirdparty/redka`。

DuckLake 没有 `ON CONFLICT`、自增、触发器、外键。适配保持现状（已在 `internal/database/kv` 落地）：

| 约束 | 做法 |
|---|---|
| 无 upsert | 应用层先按 key 做类型守卫，再在同一事务里 `MERGE` / 插入 |
| 无 rowid / sequence | 连接列就是 `"key"`；扫描游标用 key 字典序 |
| 无触发器 | `len` / `version` 由仓库在同一事务里更新 |
| 无外键级联 | 删 key = 同事务删数据表行 + `kv.keys` 行 |
| 单写 | 经 Registry 拿到该 KV catalog 的唯一 writer；`SetMaxOpenConns(1)` |

不做 stream、bitmap、HyperLogLog、geo、pub/sub、lua、RESP 端口。

### 1.1 表（不变）

```sql
CREATE SCHEMA IF NOT EXISTS kv;

-- type: 1=string 2=list 3=set 4=hash 5=zset
CREATE TABLE IF NOT EXISTS kv.keys (
    "key"   VARCHAR NOT NULL,
    "type"  SMALLINT NOT NULL,
    version BIGINT NOT NULL,
    etime   BIGINT,             -- 过期 unix ms，NULL = 永久
    mtime   BIGINT NOT NULL,
    len     BIGINT              -- string 为 NULL
);

CREATE TABLE IF NOT EXISTS kv.strings ("key" VARCHAR NOT NULL, value BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS kv.lists   ("key" VARCHAR NOT NULL, pos DOUBLE NOT NULL, elem BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS kv.sets    ("key" VARCHAR NOT NULL, elem BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS kv.hashes  ("key" VARCHAR NOT NULL, field VARCHAR NOT NULL, value BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS kv.zsets   ("key" VARCHAR NOT NULL, elem BLOB NOT NULL, score DOUBLE NOT NULL);
```

语义保持：类型不符 → `ErrKeyType`；过期 key 视为不存在；list/set/hash/zset 删空则删 key；读路径带 `etime IS NULL OR etime > now`。值在 v1 只接受 UTF-8 字符串。

---

## 2. 每个项目一份 KV catalog

KV 不是用户数据库里的 schema，也不是系统库里的表。

建项目成功后，内部再创建**一行** catalog 记录：

| 字段 | 值 |
|---|---|
| `kind` | `kv`（新增常量，与 `user` / `system` 并列） |
| `name` | `kv`（项目内保留名） |
| 引擎 | 与用户库相同的 DuckLake（Registry、descriptor、对象前缀都走现有建库路径） |

然后对该连接执行 `kv.EnsureSchema`。

要点：

- `catalog.Service.CreateDatabase` 今天把 `Kind` 固定写成 `user`。KV 要用内部入口（例如 `CreateKVDatabase`），写入 `kind=kv`，走同一套 descriptor / `SetDatabaseReady`。不要复用「用户建库」handler。
- `ListDatabases` 已按 `kind=user` 过滤，KV 行不会出现在数据库管理页。`GetDatabase` 对普通项目若被用来打开这行，按不存在处理（KV 只走 §5 的项目路由）。
- 用户建库的名字 `kv` 拒绝（`invalid_database_name` / 保留名），避免和内部行的 `(project_id, name)` 唯一约束打架。
- **用户库不再初始化 `kv` schema。** 删掉 `CreateDatabase` 成功后的 `KVInit`，删掉建项目时创建 `default` 用户库以及响应字段 `default_database`。
- 系统项目 `sb-admin` 不建 KV。对该项目调用 KV 接口返回 404 `kv_not_found`（这里的 404 表示项目没有 KV，不是某个 key 缺失）。
- DevMode 种子项目 `dev-shop` 同样建这份 catalog，本地直连后端即可用。
- 初始化失败不回滚项目：项目仍 201，打 warn。下一次 KV 请求再尝试补建（幂等 `EnsureSchema`）。控制台在补建失败时提示「Key-Value 未就绪」，不引导用户去建数据库。
- 只读实例不建 catalog。写命令返回 `ErrWriterUnavailable`。

Sweeper 只扫 `kind=kv` 且当前已打开的 catalog，每 60s `DeleteExpired`。不打开用户库，也不在只读请求里建表。

进程内写锁的粒度是这份 KV catalog 的 database id（Registry 现有模型），不是用户库 id。

---

## 3. HTTP：一种 JSON，两种 type

路由挂在项目组上，**没有** `databaseID`：

```
POST /v1/projects/:projectID/kv
Content-Type: application/json
Authorization: Bearer <API_KEY>
```

权限沿用数据面：读命令要 `database:read`，写命令要 `database:write`。admin 角色只有读权限，写命令拒绝。系统库保护与此无关（KV 不在系统库上）。

`KVHandler == nil`（无 registry）时不挂载。

一次请求只执行**一条**命令。不支持管道、`MULTI`/`EXEC`。

### 3.1 请求

`type=cmd`：`argvs` 是非空字符串数组。`argvs[0]` 是命令名（大小写不敏感），其余是参数，顺序与 Redis 一致。

```json
{ "type": "cmd", "argvs": ["SET", "session:1001", "hello", "EX", "60"] }
```

```json
{ "type": "cmd", "argvs": ["HGETALL", "user:42"] }
```

`type` 为数据类型时：`args` 是该类型的一份数据，用于添加或写入。类型在这一刻确定；key 已存在且类型不同 → 409 `kv_type_mismatch`（Redis `WRONGTYPE`）。

```json
{
  "type": "String",
  "args": { "key": "session:1001", "value": "hello", "ttl_ms": 60000 }
}
```

```json
{
  "type": "Hash",
  "args": { "key": "user:42", "fields": { "name": "alice", "age": "30" } }
}
```

```json
{ "type": "List", "args": { "key": "queue:mail", "elems": ["a", "b"], "side": "back" } }
```

```json
{ "type": "Set", "args": { "key": "tag:hot", "elems": ["go", "redis"] } }
```

```json
{
  "type": "ZSet",
  "args": { "key": "rank:score", "items": [{ "elem": "alice", "score": 10 }] }
}
```

`type` 只接受这六个字面量：`cmd`、`String`、`Hash`、`List`、`Set`、`ZSet`。其它（含小写 `string`）→ 400 `kv_invalid_argument`。

数据类型写入的对应命令：

| `type` | 等价命令 | `args` |
|---|---|---|
| `String` | `SET` | `key`、`value` 必填。可选 `ttl_ms`（毫秒，对应 `PX`）、`nx`、`xx`、`keep_ttl`。未给 `ttl_ms` 且未 `keep_ttl` 时与 Redis `SET` 一样会清掉原 TTL |
| `Hash` | `HSET` | `key`、`fields`（对象，至少一个字段）。合并字段，不删除未出现的字段 |
| `List` | `RPUSH` / `LPUSH` | `key`、`elems`（非空数组）。`side` 为 `back`（默认）或 `front`。这是追加，不是整表覆盖 |
| `Set` | `SADD` | `key`、`elems`（非空数组） |
| `ZSet` | `ZADD` | `key`、`items`（非空，元素为 `{elem, score}`） |

`Hash` / `List` / `Set` / `ZSet` 可选 `ttl_ms`：数据写成功后再 `PEXPIRE`。不给则保持原 TTL（与 Redis 这些命令一致）。`ttl_ms = 0` 表示写完后立即过期删除。

条件不满足（`nx`/`xx`）时成功响应的结果是 JSON `null`，与 Redis `SET NX` 返回 nil 一致，不是 409。

### 3.2 成功响应

HTTP 200，body 就是这条命令的 Redis 回复，编码成 JSON，**不再包一层** `{result: ...}`。

| Redis 回复 | JSON |
|---|---|
| simple string，如 `OK` | `"OK"` |
| bulk string | JSON 字符串 |
| nil | `null` |
| integer | JSON 数字 |
| array | JSON 数组；元素递归用同一规则 |

示例：

```json
"OK"
```

```json
null
```

```json
["name", "alice", "age", "30"]
```

```json
["session:1002", ["session:1001", "session:1002"]]
```

最后一行是 `SCAN`：`[下一游标, [本页 key...]]`。游标用完时为 `"0"`。

### 3.3 `cmd` 命令集

命令名大小写不敏感。参数个数不对、选项不认识 → 400 `kv_invalid_argument`。未列出的命令 → 400 `kv_unknown_command`。

整数参数按十进制解析。`EX`/`TTL`/`EXPIRE`/`EXPIREAT` 的单位是**秒**；`PX`/`PTTL`/`PEXPIRE`/`PEXPIREAT`/`ttl_ms` 的单位是**毫秒**。时间在应用层计算后写入 `etime`，SQL 里不用 `now()`。

缺失 key 遵循 Redis，不返回 404：

| 命令 | 缺失时 |
|---|---|
| `GET` 及各类读单值 | `null` |
| `EXISTS` | `0`（多 key 时只计存在的） |
| `TTL` / `PTTL` | `-2`；存在但无过期为 `-1` |
| `TYPE` | `"none"` |
| `DEL` / `EXPIRE` / `PEXPIRE` / `PERSIST` | `0` |
| `SCAN` | 空页，游标 `"0"` |

**key**

| 命令 | 说明 |
|---|---|
| `DEL key [key ...]` | 返回删除个数 |
| `EXISTS key [key ...]` | |
| `EXPIRE` / `PEXPIRE` / `EXPIREAT` / `PEXPIREAT` | 返回 1 或 0 |
| `PERSIST` | |
| `TTL` / `PTTL` | |
| `TYPE` | `string` `list` `set` `hash` `zset` 或 `none` |
| `RENAME` / `RENAMENX` | 源不存在 → 404 `kv_not_found`（Redis 的 `no such key` 是错误）。`RENAMENX` 目标已存在 → 0，不是错误 |
| `DBSIZE` | 未过期 key 数 |
| `SCAN cursor [MATCH pattern] [COUNT count] [TYPE type]` | 见下 |

`SCAN`：`cursor` 为 `"0"` 表示从头。返回的下一游标是本页最后一个 key；没有下一页时返回 `"0"`。`MATCH` 传到 DuckDB `GLOB`（`*`、`?`、`[abc]`），必须参数化，禁止拼进 SQL。`COUNT` 默认 100，上限 500。`TYPE` 取存储类型名。这与 Redis 整数游标不同，调用方把返回的游标原样传回即可。

**string**：`GET` `SET` `SETNX` `GETSET` `MGET` `MSET` `INCR` `INCRBY` `DECR` `DECRBY` `INCRBYFLOAT`。

`SET` 支持 `NX` `XX` `EX` `PX` `EXAT` `PXAT` `KEEPTTL` `GET`。`SETNX` = `SET NX`。`GETSET` = `SET` + `GET`。`MGET` 的数组与 key 对齐，缺失为 `null`。

**hash**：`HGET` `HMGET` `HSET` `HDEL` `HGETALL` `HINCRBY` `HLEN` `HSCAN`。

`HGETALL` / `HSCAN` 的数组成员是扁平的 field、value 交替（Redis 数组，不是对象）。`HLEN` 读 `kv.keys.len`。`HSCAN` 游标规则同 `SCAN`，匹配的是 field。

**list**：`LPUSH` `RPUSH` `LPOP` `RPOP` `LRANGE` `LSET` `LTRIM` `LLEN`。

`LRANGE` 支持负下标。`LLEN` 读 `len`。不做 `BLPOP`。

**set**：`SADD` `SREM` `SISMEMBER` `SMEMBERS` `SPOP` `SCARD` `SUNION` `SINTER` `SDIFF` `SUNIONSTORE` `SINTERSTORE` `SDIFFSTORE`。

无 `dest` 的集合运算是读；`*STORE` 是写。

**zset**：`ZADD` `ZREM` `ZSCORE` `ZRANK` `ZRANGE` `ZRANGEBYSCORE` `ZINCRBY` `ZCOUNT` `ZCARD`。

`ZRANGE key start stop [REV] [WITHSCORES]`。`REV` 走现有按 rank 的降序。`WITHSCORES` 时数组为 member、score 交替，score 用 JSON 数字。`ZRANGEBYSCORE` 支持 `LIMIT offset count`。

### 3.4 错误

成功与「Redis 意义上的空」都是 200。只有命令本身失败才走 `WriteError`：

| HTTP | code | 何时 |
|---|---|---|
| 400 | `kv_invalid_argument` | JSON 不合法、type 不认识、缺字段、参数类型/个数错误、`ttl_ms < 0` |
| 400 | `kv_unknown_command` | `argvs[0]` 不在 §3.3 |
| 400 | `kv_invalid_value` | `INCR` / `HINCRBY` 等的现值不是数 |
| 404 | `kv_not_found` | 项目没有 KV catalog，或 `RENAME` 的源 key 不存在 |
| 409 | `kv_type_mismatch` | `WRONGTYPE` |
| 403 | `system_database_protected` | 不用于 KV。系统项目直接 404 |
| 403 / 现有码 | writer unavailable | 实例只读时的写命令 |

读/写划分：`GET` `MGET` `EXISTS` `TTL` `PTTL` `TYPE` `SCAN` `DBSIZE` `HGET` `HMGET` `HGETALL` `HLEN` `HSCAN` `LRANGE` `LLEN` `SISMEMBER` `SMEMBERS` `SCARD` `SUNION` `SINTER` `SDIFF` `ZSCORE` `ZRANK` `ZRANGE` `ZRANGEBYSCORE` `ZCOUNT` `ZCARD` 为读，其余为写。`SET NX` 即使没写成也是写。

### 3.5 Handler 接线

一个 `Execute` 即可，不再为每种命令注册一条路由。

```
解析项目 → 取 kind=kv 的 catalog 行（没有则尝试补建一次）
→ 系统项目直接 404
→ 按命令分类做 writable / 权限检查
→ Acquire 该 catalog → kv.New(db, OnWrite)
→ type=cmd：解析 argvs，调已有仓库方法
→ type=数据类型：校验 args，调对应的 Set / HSET / Push / SADD / ZADD
→ 写事务提交成功后 NotifyWrite（与 SQL 写路径同一条 catalog 同步）
→ 200 + JSON 回复
```

`argvs` 与 `args` 互斥：`cmd` 禁止带 `args`，数据类型禁止带 `argvs`。

dispatcher 是一张命令表（名字 → 解析函数 → 仓库方法），不要把命令字符串拼成 SQL。占位符全部走现有仓库。

`internal/database/kv` 的导出方法已经覆盖 §3.3，dispatcher 不新增表、不改 `MERGE` 模板。缺的只是参数解析（`EX` 秒换毫秒、`SCAN` 选项、`ZRANGE REV`）。

旧的 `kv_handler.go` 里按资源拆开的方法、`kv_types.go` 里那些路径专用请求体、`router.go` 里 `/databases/:databaseID/kv/...` 整组，删掉，改成这一条路由。

---

## 4. 控制台

Key-Value 是左侧导航的项目页（`ui/src/pages/KeyValue.vue`），跟随当前项目。页面上没有数据库选择器，也不读取数据库列表。

数据库管理页只保留集合文档。去掉展开行里的 Key-Value 页签；若 `DataTabs` 只为这个页签存在，展开行直接渲染原来的集合面板。

页面内仍是两个子页签：

| 子页签 | 行为 |
|---|---|
| 数据 | `SCAN` / 按类型查看 / 新建 / 编辑 / TTL / 重命名 / 删除。全部走 `POST .../kv`，读和结构命令用 `type=cmd`，新建和按类型保存用 `String` 等 `args` |
| API | 只说明这一个 URL，并给出 `cmd` 与五种 `type` 的可复制 JSON。不再罗列旧的 REST 路径表 |

新建弹窗：先选类型（String / Hash / List / Set / ZSet），再填该类型的初始数据，提交对应的 `args`。类型冲突 toast 409，不覆盖。

空态文案是「暂无 Key」，按钮是新建。未就绪时说明 KV 会随项目准备，而不是让用户去建数据库。

前端 `api.kv` 收成项目级的一次调用（mock 与 http 同一签名），例如 `api.kv.exec(projectId, body)`，返回解析后的 JSON。按 databaseId 拆开的 `getString` / `setHash` / … 删除。mock 按项目存一份内存 KV，不按数据库分桶。

---

## 5. 持久化

与用户库的 DuckLake 写路径相同，只是快照落在项目的 KV catalog 上：

1. 写返回时本地 catalog 已提交；对象存储约 200ms debounce 异步同步。
2. 每条写命令一个快照。高频 `INCR` 会堆积版本；能用 `MSET` / 多 field `HSET` 的不要拆成多次 HTTP。
3. 小行仍走 `data_inlining_row_limit` 内联。
4. 删除与 TTL 清扫是 merge-on-read；Sweeper 只扫 KV catalog。
5. 建议规模（超限不报错，变慢）：单 key 元素 ≤ 10k，value ≤ 1MB，单项目 key ≤ 100k。

---

## 6. 测试与收尾

1. 仓库单测保持：它们不关心 HTTP 是否带 databaseId。
2. 建项目后存在且仅存在一行 `kind=kv`，`HasSchema` 为 true，用户库列表里看不到它。用户库上没有 `kv` schema。建库 handler 不再调用 `EnsureSchema`。
3. `POST /projects/:id/kv`：`cmd` 的 `GET` 缺失为 `null` 且 200；`SET`/`HSET` 类型冲突 409；`SCAN` 游标；只读实例拒绝写；系统项目 404；未知命令 400。
4. 五种 `type` 的 `args` 能创建对应类型，再用 `cmd` `TYPE` 读回。
5. UI：Key-Value 页无数据库下拉；新建 Hash 走 `type=Hash`；数据库页展开行没有 Key-Value 页签。
6. `docs/database/kv.md` 的示例与 §3 一致。

`go build ./internal/... ./cmd/... && go test ./internal/...`，以及 UI `npm run test`。

---

## 7. 仍不做

- RESP / redis-cli 直连。HTTP JSON 是 v1 的协议。
- 一次请求多条命令、`MULTI`/`EXEC`。
- stream、bitmap、HyperLogLog、geo、pub/sub、lua、`KEYS`、阻塞弹出。
- 二进制 value（无 base64 通道）。
- 把 KV 放进用户自建数据库，或在控制台里为 KV 选择数据库。
