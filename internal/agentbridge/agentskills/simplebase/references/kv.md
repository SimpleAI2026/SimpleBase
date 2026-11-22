# kv
<!-- allow: kv -->

命令：`kv exec --command NAME [--arg VALUE ...]`。项目级单端点，Redis 语义，命令名大小写不敏感，`--arg` 按位置顺序出现 0–N 次。

## exec

```
simplebase kv exec --command SET --arg session:1 --arg ok --arg EX --arg 3600
simplebase kv exec --command GET --arg session:1
simplebase kv exec --command SCAN --arg 0 --arg MATCH --arg session:* --arg COUNT --arg 100
simplebase kv exec --command INCR --arg counter:visits
simplebase kv exec --command HSET --arg user:1 --arg name --arg alice
simplebase kv exec --command DEL --arg session:1   ← 需要确认
```

必填：`--command`。`EX`、`NX`、`MATCH`、`COUNT` 等选项也是普通 `--arg`，写在它所属的位置参数之后。

## 命令范围

- 键：`DEL EXISTS EXPIRE PEXPIRE EXPIREAT PEXPIREAT PERSIST TTL PTTL TYPE RENAME RENAMENX DBSIZE SCAN`
- 字符串：`GET SET SETNX GETSET MGET MSET INCR INCRBY DECR DECRBY INCRBYFLOAT`
- 哈希：`HGET HMGET HSET HDEL HGETALL HINCRBY HLEN HSCAN`
- 列表：`LPUSH RPUSH LPOP RPOP LRANGE LSET LTRIM LLEN`
- 集合：`SADD SREM SISMEMBER SMEMBERS SPOP SCARD SUNION SINTER SDIFF SUNIONSTORE SINTERSTORE SDIFFSTORE`
- 有序集合：`ZADD ZREM ZSCORE ZRANK ZRANGE ZRANGEBYSCORE ZINCRBY ZCOUNT ZCARD`

没有 `KEYS`，枚举键用 `SCAN`：游标从 `0` 开始，回复的游标为 `0` 表示扫完；`COUNT` 取 1–500。

## 确认与错误

- 只有 `DEL` 需要界面确认；headless 返回 `confirmation_unavailable`，不要重试。
- `kv_unknown_command`：命令不在上表，换命令，不要原样重试。
- `kv_invalid_argument`：参数个数或选项不对，对照上表修正。
- `kv_type_mismatch`：键已是别的类型，先 `TYPE` 确认。
- `kv_invalid_value`：对非数字值做了 `INCR` 之类数值命令。
- `writer_unavailable`：实例只读，写命令不可用。
