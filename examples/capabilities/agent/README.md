# 云助手：自然语言、助手技能、SSE、删除确认

这些用例不直接拼 CLI。脚本登录后的访问令牌调用控制台同一条运行接口：

`POST /v1/projects/:projectID/agent-threads/:threadID/runs`

请求体 `stream: true`。`@` 只用于点名助手，**不能**用它选技能：每个用例在 `cases.json` 里用 `module` 指定要用的助手，脚本按 `GET /agents` 找到该 module 的内置助手并建线程，技能由助手 module 在服务端自动加载。响应是 SSE。破坏性命令出现 `confirmation_required` 时，脚本向

`POST /v1/projects/:projectID/agent-runs/:runID/confirmations/:callID`

提交 `{"approve": true}`，然后继续读流，直到 `end`。

内置助手默认 `writable=false`。第一条用例在未打开写权限时要求建库，预期 CLI 被 `write_not_allowed` 拒绝，并且不会弹出确认。随后脚本把对应用例的助手设为可写，再跑建表、文档删除、KV 删除、云函数、对象删除和定时任务。

`kv` / `gofunction` / `cron` 三个助手只在服务端启用 CLI 技能（`skills.cli`）时才会播种；缺少对应助手时脚本会记一条失败并提示原因。`project` 模块没有对应助手，因此不再有用例，其命令前缀白名单由 `internal/api` 的单元测试覆盖。

模型是服务端 `.env` 里的 `SIMPLEBASE_LLM_PROVIDER_DEFAULT_MODEL`。结果以「是否调用了 simplebase CLI」和「调用之后资源是否达到目标」为准，不以回复措辞为准。

```bash
export SIMPLEBASE_URL=http://127.0.0.1:8080
export SIMPLEBASE_TOKEN="$ACCESS_TOKEN"
export SIMPLEBASE_PROJECT_ID="$PROJECT_ID"
export SIMPLEBASE_BIN=./simplebase
export RUN_ID=demo01
export RESULT_FILE=/tmp/sb-results.jsonl
node examples/capabilities/agent/run.mjs
```

指令全文在 `cases.json`。本地 `gocloudagent` 二进制是另一条命令面（本机 shell / 文件工具，headless 下破坏性命令得不到界面确认），由 `examples/run_agent.sh` 额外探一次，不替代上面的 SSE 用例。

本地 `gocloudagent` 的 shell 工具会剥离 `SIMPLEBASE_*` / `GOCLOUDAGENT_*` 与凭据类环境变量，所以 shell 里的 `simplebase` 会返回 `config_missing`（退出码 4）。这条探针改为事先写好 `$XDG_CONFIG_HOME/simplebase/config.json`（权限 `0600`），并在进程结束后删除该目录。详见 `docs/cli.md`。
