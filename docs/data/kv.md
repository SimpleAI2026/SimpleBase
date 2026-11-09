---
title: Key-Value
order: 10
group: Key-Value
---

# Key-Value

每个项目自带一份 Key-Value，创建项目后即可使用。它不在某个数据库里，控制台也不需要选择数据库。

语义与 Redis 的 key、string、hash、list、set、zset 对齐。底层与项目数据库共用同一个单文件（内部表以 `__sb_` 为前缀，用户 SQL 无法访问），调用方只使用下面这一条 HTTP。

## 请求

```http
POST /v1/projects/:projectId/kv
Authorization: Bearer <API_KEY>
Content-Type: application/json
```

一次请求执行一条命令。读命令需要 `database:read`，写命令需要 `database:write`。只读 Key 可执行读命令，不能执行写命令或按类型写入。

`type` 决定正文：

| type | 正文 | 作用 |
|---|---|---|
| `cmd` | `argvs`：字符串数组，第一项是命令 | 任意已支持的 Redis 命令 |
| `String` `Hash` `List` `Set` `ZSet` | `args`：该类型的数据 | 按类型写入一份 KV |

`type` 必须是上表里的字面量（`String` 不能写成 `string`）。

### 用命令

`argvs[0]` 是命令名，大小写不敏感，后面是参数，顺序与 Redis 相同。

```bash
curl -X POST "$BASE/v1/projects/$PROJECT/kv" \
  -H "Authorization: Bearer $SB_KEY" \
  -H "Content-Type: application/json" \
  -d '{"type":"cmd","argvs":["SET","session:1001","hello","EX","60"]}'
```

```bash
curl -X POST "$BASE/v1/projects/$PROJECT/kv" \
  -H "Authorization: Bearer $SB_KEY" \
  -H "Content-Type: application/json" \
  -d '{"type":"cmd","argvs":["GET","session:1001"]}'
```

```bash
curl -X POST "$BASE/v1/projects/$PROJECT/kv" \
  -H "Authorization: Bearer $SB_KEY" \
  -H "Content-Type: application/json" \
  -d '{"type":"cmd","argvs":["HSET","user:42","name","alice","age","30"]}'
```

### 按类型写入

写入时指定类型。key 已经是另一种类型时返回 409，不会覆盖。

String（`SET`）：

```json
{
  "type": "String",
  "args": { "key": "session:1001", "value": "hello", "ttl_ms": 60000 }
}
```

`args` 里 `key`、`value` 必填。可选：

| 字段 | 含义 |
|---|---|
| `ttl_ms` | 过期毫秒数，等价于 `SET` 的 `PX` |
| `nx` | 仅当 key 不存在时写入 |
| `xx` | 仅当 key 存在时写入 |
| `keep_ttl` | 保留原来的过期时间 |

未设置 `ttl_ms` 且未设置 `keep_ttl` 时，这次 `SET` 会清掉原 TTL。`nx` / `xx` 条件不满足时 HTTP 仍是 200，正文为 `null`。

Hash（`HSET`，合并字段）：

```json
{
  "type": "Hash",
  "args": {
    "key": "user:42",
    "fields": { "name": "alice", "age": "30" }
  }
}
```

List（追加，默认尾部 `RPUSH`）：

```json
{
  "type": "List",
  "args": { "key": "queue:mail", "elems": ["a", "b"], "side": "back" }
}
```

`side` 取 `back` 或 `front`。

Set（`SADD`）：

```json
{
  "type": "Set",
  "args": { "key": "tag:hot", "elems": ["go", "redis"] }
}
```

ZSet（`ZADD`）：

```json
{
  "type": "ZSet",
  "args": {
    "key": "rank:score",
    "items": [{ "elem": "alice", "score": 10 }]
  }
}
```

Hash、List、Set、ZSet 可以额外带 `ttl_ms`：数据写完后再设置过期。不带则保留原来的 TTL。`ttl_ms` 为 `0` 表示写完即过期删除。

## 响应

成功时 HTTP 200，正文就是这条命令的 Redis 回复，JSON 编码，没有外包一层。

| 回复 | 正文示例 |
|---|---|
| 状态 | `"OK"` |
| 字符串 | `"hello"` |
| 空（key 不存在等） | `null` |
| 整数 | `2` |
| 数组 | `["name","alice","age","30"]` |

`HGETALL` 是 field、value 交替的扁平数组。`ZRANGE ... WITHSCORES` 是 member、score 交替，score 为 JSON 数字。

key 不存在时读命令返回空值，**不是** 404。例如 `GET` 为 `null`，`EXISTS` 为 `0`，`TYPE` 为 `"none"`，`TTL` / `PTTL` 为 `-2`。

### SCAN

```json
{ "type": "cmd", "argvs": ["SCAN", "0", "MATCH", "session:*", "COUNT", "100", "TYPE", "string"] }
```

响应：

```json
["session:1002", ["session:1001", "session:1002"]]
```

第一项是下一页游标，把它原样放进下一次 `SCAN`。扫完时游标为 `"0"`。`MATCH` 支持 `*`、`?`、`[abc]`。`COUNT` 默认 100，最大 500。`TYPE` 可选 `string`、`list`、`set`、`hash`、`zset`。

## 命令

时间单位：`EX`、`EXPIRE`、`EXPIREAT`、`TTL` 是秒；`PX`、`PEXPIRE`、`PEXPIREAT`、`PTTL` 以及 `ttl_ms` 是毫秒。

### key

`DEL` `EXISTS` `EXPIRE` `PEXPIRE` `EXPIREAT` `PEXPIREAT` `PERSIST` `TTL` `PTTL` `TYPE` `RENAME` `RENAMENX` `SCAN` `DBSIZE`

`RENAME` 的源 key 不存在时是 404。`RENAMENX` 在目标已存在时返回 `0`。

### string

`GET` `SET` `SETNX` `GETSET` `MGET` `MSET` `INCR` `INCRBY` `DECR` `DECRBY` `INCRBYFLOAT`

`SET` 可选 `NX` `XX` `EX` `PX` `EXAT` `PXAT` `KEEPTTL` `GET`。

```json
{ "type": "cmd", "argvs": ["MGET", "a", "b"] }
```

```json
["hello", null]
```

### hash

`HGET` `HMGET` `HSET` `HDEL` `HGETALL` `HINCRBY` `HLEN` `HSCAN`

### list

`LPUSH` `RPUSH` `LPOP` `RPOP` `LRANGE` `LSET` `LTRIM` `LLEN`

`LRANGE` 支持负下标。

### set

`SADD` `SREM` `SISMEMBER` `SMEMBERS` `SPOP` `SCARD` `SUNION` `SINTER` `SDIFF` `SUNIONSTORE` `SINTERSTORE` `SDIFFSTORE`

### zset

`ZADD` `ZREM` `ZSCORE` `ZRANK` `ZRANGE` `ZRANGEBYSCORE` `ZINCRBY` `ZCOUNT` `ZCARD`

`ZRANGE` 可选 `REV`、`WITHSCORES`。`ZRANGEBYSCORE` 可选 `LIMIT offset count`。

## 错误

| HTTP | code | 含义 |
|---|---|---|
| 400 | `kv_invalid_argument` | JSON、`type`、字段或参数不合法 |
| 400 | `kv_unknown_command` | 不支持的命令 |
| 400 | `kv_invalid_value` | 自增时当前值不是数字 |
| 404 | `kv_store_not_found` | 该项目没有 Key-Value（系统项目，或只读实例下无法补建） |
| 404 | `kv_not_found` | `RENAME` 的源 key 不存在 |
| 409 | `kv_type_mismatch` | key 已是别的类型（`WRONGTYPE`） |

## 行为说明

- 一个 key 同时只能是一种类型。list、set、hash、zset 的元素被删空后，key 一并删除。
- 过期 key 在读和写时都当作不存在。实例大约每 60 秒清理一次已过期数据。
- 值是 UTF-8 文本。
- 写成功表示本机 catalog 已提交。对象存储上的副本由后台在大约 200ms 内同步，与 SQL 写入相同。
- 每次写生成一个存储快照。计数器一类的高频更新尽量合并（例如 `MSET`、一次 `HSET` 多个字段），避免把一条逻辑拆成很多次请求。
- 建议规模：单个 key 的元素不超过 1 万，单个值不超过 1MB，单个项目的 key 不超过 10 万。超出后仍可读写，只是变慢。

## 不做

- 不提供 Redis 端口，不能用 redis-cli 直连。
- 一条 HTTP 只跑一条命令，没有 `MULTI` / `EXEC`，也没有管道。
- 没有 stream、bitmap、HyperLogLog、geo、pub/sub、Lua、`KEYS`、阻塞弹出（`BLPOP` 等）。

## 使用场景

下面每个例子都是一条或几条 `POST /v1/projects/:projectID/kv` 请求。为简洁起见定义一个 shell 函数：

```bash
kv() {
  curl -sS -X POST "$BASE/v1/projects/$PROJECT/kv" \
    -H "Authorization: Bearer $SB_KEY" -H 'Content-Type: application/json' \
    -d "{\"type\":\"cmd\",\"argvs\":$1}"
}
```

### 场景一：会话缓存与过期

登录后把会话写进 KV，60 秒后自动失效，续期用 `EXPIRE`：

```bash
kv '["SET","session:u-42","{\"uid\":42}","EX","60"]'
kv '["GET","session:u-42"]'
kv '["EXPIRE","session:u-42","300"]'
kv '["TTL","session:u-42"]'
```

key 过期后 `GET` 返回 `null`，不是 404。

### 场景二：接口限流计数

固定窗口限流：第一次访问时设置窗口，之后只自增。

```bash
kv '["INCR","rate:u-42:2026-10-10T10:00"]'
kv '["EXPIRE","rate:u-42:2026-10-10T10:00","60"]'
```

`INCR` 返回当前计数，应用层判断是否超过阈值。注意 `INCR` 与 `EXPIRE` 是两条独立请求，没有事务；更严格的限流应在应用层合并窗口，或把计数放进一次 `MSET`/`HSET` 等批量写，减少请求数。

### 场景三：幂等与简易分布式锁

`SET ... NX EX` 只在 key 不存在时写入。返回 `"OK"` 表示抢到，返回 `null` 表示已被占用：

```bash
kv '["SET","lock:order:o-1001","worker-a","NX","EX","30"]'
```

抢到后再去写数据库，处理完用 `DEL` 释放。该锁适合单项目内的防重复提交，不是跨系统的强一致锁；锁过期时间必须大于业务处理耗时。

### 场景四：排行榜

用 ZSET 保存分数，`ZINCRBY` 增分，`ZRANGE ... REV WITHSCORES` 取前几名：

```bash
kv '["ZADD","rank:weekly","120","alice"]'
kv '["ZINCRBY","rank:weekly","30","alice"]'
kv '["ZADD","rank:weekly","95","bob"]'
kv '["ZRANGE","rank:weekly","0","9","REV","WITHSCORES"]'
```

返回的是 member、score 交替的扁平数组，例如 `["alice","150","bob","95"]`（score 为 JSON 数字）。

### 场景五：轻量任务队列

生产端 `LPUSH`，消费端 `RPOP`（没有阻塞弹出，需轮询）：

```bash
kv '["LPUSH","queue:mail","{\"to\":\"a@example.com\"}"]'
kv '["RPOP","queue:mail"]'
kv '["LLEN","queue:mail"]'
```

队列为空时 `RPOP` 返回 `null`。需要可靠投递、重试或消费确认的场景请使用数据库表或云函数加定时任务，不要把 KV 当作消息中间件。

### 场景六：用户资料缓存（Hash）

```bash
kv '["HSET","user:42","name","alice","age","30"]'
kv '["HGETALL","user:42"]'
kv '["HINCRBY","user:42","age","1"]'
```

### 场景七：只读 Key 能做什么

只带 `database:read` 的 Key 可以执行 `GET`、`HGETALL`、`ZRANGE`、`SCAN` 等读命令；执行 `SET`、`INCR`、`DEL` 等写命令会返回 403，适合给前台服务或报表只读使用。

## 常见问题

| 现象 | 可能原因 | 处理 |
|---|---|---|
| 返回 409 `kv_type_mismatch` | 该 key 已经是别的类型 | 先 `TYPE` 查看，必要时 `DEL` 后重建，或换一个 key |
| 返回 400 `kv_unknown_command` | 命令不在支持列表里 | 对照上文「命令」清单，例如没有 `KEYS`，请用 `SCAN` |
| `INCR` 返回 400 `kv_invalid_value` | 当前值不是数字 | 检查该 key 里存的值 |
| 返回 503 `writer_unavailable` | 当前实例不是可写实例 | 写命令只能打到写实例 |
| 设置了 `SET` 后过期时间丢失 | `SET` 未带 `EX`/`PX` 会清掉原 TTL | 续写时带上 `KEEPTTL` 或重新指定过期时间 |
