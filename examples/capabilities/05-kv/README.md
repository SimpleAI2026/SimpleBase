# Key-Value

项目级 KV，Redis 语义。命令名和参数都用 `--command` / `--arg`，不要拼成一条 shell 字符串。

## 运行

```bash
export CASE=05-kv
./examples/capabilities/05-kv/run.sh
```

## 命令

```bash
simplebase kv exec --command SET --arg "ex:$RUN_ID:color" --arg blue
simplebase kv exec --command GET --arg "ex:$RUN_ID:color"
simplebase kv exec --command INCR --arg "ex:$RUN_ID:visits"
simplebase kv exec --command HSET --arg "ex:$RUN_ID:user" --arg name --arg alice
simplebase kv exec --command HGET --arg "ex:$RUN_ID:user" --arg name
simplebase kv exec --command DEL --arg "ex:$RUN_ID:color"
simplebase kv exec --command GET --arg "ex:$RUN_ID:color"
```

只有 `DEL` 在云助手里需要确认。人类 CLI 直接删除。
