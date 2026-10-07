---
title: Key-Value
order: 2
---

# Key-Value

每个项目自带一份 Key-Value，创建项目后即可使用。它不在某个数据库里，控制台也不需要选择数据库。

语义与 Redis 的 key、string、hash、list、set、zset 对齐。底层是该项目专属的 DuckLake 存储，调用方只使用下面这一条 HTTP。

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
| 404 | `kv_not_found` | 该项目没有 Key-Value，或 `RENAME` 的源 key 不存在 |
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
