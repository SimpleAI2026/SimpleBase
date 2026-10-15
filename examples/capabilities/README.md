# 能力示例

按 SimpleBase 资源 CLI（`cmd/simplebase` → `internal/sbcli`）覆盖主要能力。每个目录有 `README.md`、可执行 `run.sh` 和 `expected.md`。已有的四个业务案例（`shop`、`community`、`iot-telemetry`、`ops-assistant`）仍由 `examples/init.sh` 初始化，不在这里重复。

| 目录 | 能力 |
|---|---|
| `01-project` | 创建项目、项目设置、配额、审计、日志检索、日志保留 |
| `02-auth` | 创建用户、签发只读 API Key、写操作被拒绝、吊销 |
| `03-sql` | SQL 库建表、加字段、插入、查询、更新、删除 |
| `04-documents` | 集合库、文档插入、列出、更新、删除 |
| `05-kv` | SET / GET / INCR / HSET / HGET / DEL |
| `06-functions` | 云函数创建、调用、新版本试跑、激活 |
| `07-storage` | 上传、列举、预签名、删除 |
| `08-cron` | 定时任务创建、触发、运行记录、删除 |
| `agent` | 云助手自然语言、`@` 子 skill、SSE、删除确认 |

```bash
./examples/run_all.sh --start
./examples/run_all.sh --url http://127.0.0.1:8080 --skip-agent
./examples/run_agent.sh
```

`--start` 会执行 `./build.sh`（需要已有或将生成的 `config.yaml` 与根目录 `.env`），在后台启动 `./simplebase`，跑完 CLI 与云助手后停止本次拉起的进程。存储默认本地盘。S3 只在同时具备 endpoint、region、bucket 时才有意义；缺任何一项时这些示例不切换到对象存储。

口令、API Key 和访问令牌只留在进程环境里。`RESULT_FILE`（默认 `/tmp/simplebase-examples/results.jsonl`）里的失败详情会打码。
