---
title: 云助手模型联调与验收
order: 3
---

# 云助手模型联调与验收

本页用于验证 `plan/planv4.0/cloud-agent-optimization-plan.md` 的前后端能力。默认单元测试不访问外网；真实模型的 key 只放进被 Git 忽略的 `.env` 或 CI secret。

## SiliconFlow 配置

```sh
SIMPLEBASE_LLM_ENABLED=true
SIMPLEBASE_LLM_PROVIDERS=openai
SIMPLEBASE_LLM_PROVIDER_OPENAI_API_KEY=<从 SiliconFlow 控制台取得的密钥>
SIMPLEBASE_LLM_PROVIDER_OPENAI_BASE_URL=https://api.siliconflow.cn/v1
SIMPLEBASE_LLM_PROVIDER_OPENAI_DEFAULT_MODEL=deepseek-ai/DeepSeek-V4-Flash
SIMPLEBASE_LLM_PROVIDER_OPENAI_ALLOWED_MODELS=deepseek-ai/DeepSeek-V4-Flash
SIMPLEBASE_LLM_PROVIDER_OPENAI_TIMEOUT=120s
SIMPLEBASE_LLM_AGENT_TOOL_PROTOCOL=auto
SIMPLEBASE_LLM_AGENT_RUN_TIMEOUT=180s
```

如果 `llm.agent_tool_protocol=auto` 遇到不支持原生 `tools` 的供应商，会在工具协议 400/422 时降级为文本工具协议；可用 `text` 强制旧协议。

## 手工 API 验收

启动服务并使用已有的项目 ID 和 API key（不要把真实 key 复制到文档或日志）：

```sh
./build.sh dev --no-open
P=<project_id>
KEY=<SimpleBase API key>
H="Authorization: Bearer $KEY"
TH=$(curl -sS -X POST "localhost:8080/v1/projects/$P/agent-threads" -H "$H" -H 'Content-Type: application/json' -d '{"title":"e2e"}' | jq -r .id)
AG=$(curl -sS "localhost:8080/v1/projects/$P/agents" -H "$H" | jq -r '.agents[] | select(.module=="database") | .id' | head -1)
curl -N -sS -X POST "localhost:8080/v1/projects/$P/agent-threads/$TH/runs" -H "$H" -H 'Content-Type: application/json' \
  -d "{\"content\":\"列出项目里的数据库\",\"mentions\":[{\"agent_id\":\"$AG\"}],\"stream\":true}"
curl -sS "localhost:8080/v1/projects/$P/agent-threads/$TH/runs?limit=20" -H "$H" | jq '.runs[0] | {status,duration_ms,prompt_tokens,completion_tokens,tool_calls}'
```

SSE 依次包含 `run`、可选 `thinking`、逐段 `token`、工具执行时同一个 `call_id` 的 `tool_call`/`tool_result`、`usage`、`end`。无响应时会有 `: ping` 保活；错误帧为固定 `code` 和文案，不包含上游错误详情。

控制台验收：创建 Agent 并填写模型；创建/切换/重命名/删除会话，刷新恢复；观察思考中、回复中和工具卡片；停止后保留已生成内容并显示「已停止」；故意断网后可点击「重试」。

## 自动化回归

```sh
go test ./internal/cloudagent ./internal/api ./internal/llmgateway ./internal/config ./internal/systemdb
cd ui && yarn build && yarn test
```

真实模型测试应以单独的 `llm_integration` 标签运行，不能加入默认 CI。`internal/api/agent_llm_integration_test.go` 的 `TestLLMRealStreamAndToolRoundtrip` 会分别检验 native/text 两种工具协议，`TestLLMModelAllowlistRejectsBeforeUpstream` 检验模型白名单；未设置 key 时跳过：

```sh
# 在当前 shell 提供以上环境变量后运行；不要在命令行写入明文 key
go test -tags llm_integration ./internal/api -run 'TestLLMRealStreamAndToolRoundtrip|TestLLMModelAllowlistRejectsBeforeUpstream' -v -timeout 5m
```

授权凭据必须经环境变量读取，不得硬编码进任何 Go/Vue 测试或计划文件。
