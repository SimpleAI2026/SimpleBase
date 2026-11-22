# gocloudagent

通用 Agent 能力模块，独立 Go module `github.com/linkxzhou/SimpleBase/gocloudagent`。模块不 import SimpleBase 根模块或任何 `internal/...` 包，可接入不同平台。

## 能力边界

- `agentcore/`：模型协议、原生/文本工具调用、流式合并、用量、脱敏。
- `cloudagent/`：通用运行循环、线程占用与取消、超时、流事件、工具错误转换、通用计划校验、协作任务图、MCP 工具装配与确认。
- `mcp/`：MCP HTTP / stdio / in-memory client、in-memory server、命名和结果转换。
- `skills/`：从任意 `fs.FS` 加载 Markdown 技能文档；不内置任何平台技能或命令白名单。
- `cli/`：本地命令行、会话、压缩、REPL、MCP 配置。
- `tools/`：本机 shell、文件、glob、grep 与 web 搜索工具。
- `cmd/`：CLI 入口，只依赖模块根包的 `RunCLI` 门面。

SimpleBase 专属能力全部位于 `internal/agentbridge`：项目与身份、数据库/KV/S3/日志工具、SimpleBase CLI 与委托令牌、模块目录、BI、固定六阶段计划、技能文档和命令授权。API 层只负责 HTTP 与持久化。

## 接入方式

`cloudagent.Runtime` 只依赖 `ChatClient` 和可选的 `RunProvider`：

```go
rt := &cloudagent.Runtime{
    LLM: client,
    Provider: cloudagent.RunProviderFunc(func(ctx context.Context, req cloudagent.RunRequest, emit func(cloudagent.Event)) (cloudagent.RunPreparation, error) {
        return cloudagent.RunPreparation{
            Model: "model-name", ModelScope: "tenant-or-workspace",
            Instruction: "You are a helpful assistant.",
            Tools: toolsFor(req.Context),
            Cleanup: func() {},
        }, nil
    }),
}
```

`RunRequest.Context` 是 provider 与 MCP connector 共有的 opaque context，核心 Runtime 不解释它。`RunPreparation` 统一承载本轮模型、提示词、工具、清理函数和工具调用回调。无 provider 时可直接使用 `Runtime.Instruction`、`Runtime.Tools` 与 `Agent` 字段。

通用计划使用 `PlanStageSpec` / `ValidatePlanStages`：阶段 ID 由接入方定义，依赖只能引用更早阶段。固定业务阶段不属于模块。

## MCP 安全

`MCPConnector` 按 `RunRequest` 建立本轮会话；凭据、租户授权和网络出口策略由接入方实现。`MCPPolicy` 对工具做只读/破坏性分类：策略缺失或出错时 fail-closed；不可写 Agent 不能调用写工具；破坏性调用需要确认，无人值守运行直接拒绝。会话随 run 逆序清理。

本机 `tools` 的 shell 开启隔离但环境不支持时返回错误，不退回裸执行。子进程不继承 `GOCLOUDAGENT_*` 或名称含 TOKEN / SECRET / PASSWORD / API_KEY 等凭据变量。

## CLI 配置

`~/.gocloudagent/config.json`：

```json
{
  "endpoint": "https://api.openai.com/v1",
  "token": "...",
  "model": "...",
  "mcp_servers": {
    "fs": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "."],
      "read_only_tools": ["read_file", "list_directory"]
    }
  }
}
```

`endpoint` 是完整的 OpenAI 兼容基础地址，客户端只追加 `/chat/completions` 或 `/models`，不拼接平台业务路径。接入 SimpleBase 时由调用方配置完整地址，例如 `http://localhost:8080/v1/projects/<project-id>/v1`。配置只读取 `GOCLOUDAGENT_*` 环境变量，不读取 `SIMPLEBASE_*`，也没有 `project_id` 字段。

## 构建与测试

```bash
cd gocloudagent
GOWORK=off go build ./...
GOWORK=off go vet ./...
GOWORK=off go test -race ./...
```
