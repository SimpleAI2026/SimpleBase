# 项目管理、设置、配额、审计与日志

用 `simplebase` CLI 创建项目，并在该项目里读写一条项目设置、读取配额、列出审计、检索日志和读取日志保留天数。

项目 ID 由服务端生成（8 位 `[A-Za-z0-9-]`）。CLI 只有 `project list` / `project create`，没有删除项目的命令。

## 运行

先登录拿到访问令牌，再执行（`run_all.sh` 会做登录并把新建项目的 ID 传给后续示例）：

```bash
export SIMPLEBASE_BIN=./simplebase
export SIMPLEBASE_URL=http://127.0.0.1:8080
export SIMPLEBASE_TOKEN="$ACCESS_TOKEN"
export SIMPLEBASE_PROJECT_ID=sb-admin
export RUN_ID=demo01
export RESULT_FILE=/tmp/sb-results.jsonl
export CASE=01-project
./examples/capabilities/01-project/run.sh
```

脚本成功后把新建项目 ID 写到 `PROJECT_ID_FILE`（默认 `/tmp/simplebase-examples/project_id`）。

## 命令

```bash
simplebase project list
simplebase project create --name "cap-$RUN_ID"
simplebase settings set --json '{"key":"examples.note","value_json":"\"cap\""}'
simplebase settings get
simplebase quota get
simplebase audit list --limit 5
simplebase log search --limit 5
simplebase log retention get
```

用户、API Key 与实例设置不在云助手 skill 里，只能由人类操作者的 CLI 执行。
