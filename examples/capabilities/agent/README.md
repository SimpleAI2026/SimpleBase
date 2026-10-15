# 云助手：自然语言、子 skill、SSE、删除确认

这些用例不直接拼 CLI。脚本登录后的访问令牌调用控制台同一条运行接口：

`POST /v1/projects/:projectID/agent-threads/:threadID/runs`

请求体 `stream: true`，正文里写 `@数据库`、`@云函数` 等别名。脚本按控制台同样的别名表解析出 `skills`，与正文一起提交。响应是 SSE。破坏性命令出现 `confirmation_required` 时，脚本向

`POST /v1/projects/:projectID/agent-runs/:runID/confirmations/:callID`

提交 `{"approve": true}`，然后继续读流，直到 `end`。

内置助手默认 `writable=false`。第一条用例在未打开写权限时要求建库，预期 CLI 被 `write_not_allowed` 拒绝，并且不会弹出确认。随后脚本把「通用助手」设为可写，再跑建表、文档删除、KV 删除、云函数、对象删除、配额和定时任务。

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

指令全文在 `cases.json`。本地 `cloudagent` 二进制是另一条命令面（本机 shell / 文件工具，headless 下破坏性命令得不到界面确认），由 `examples/run_agent.sh` 额外探一次，不替代上面的 SSE 用例。
