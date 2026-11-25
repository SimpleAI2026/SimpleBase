# gocloudagent

通用 Agent 能力模块，独立 Go module `github.com/linkxzhou/SimpleBase/gocloudagent`。模块不 import SimpleBase 根模块或任何 `internal/...` 包，可接入不同平台。

## 能力边界

| 包 | 职责 |
| --- | --- |
| `agentcore/` | 模型协议（native / text 回退、stream→chat 退化）、流式合并、用量、脱敏、工具结果编码（`MarshalToolJSON` / `ErrorEnvelope` / `ToolResultIsError`）。 |
| `tools/` | 本机工具：`bash` / `read` / `write` / `edit` / `glob` / `grep`（`Workspace`，路径与符号链接防护、隔离、有界输出）、`http` / `web_fetch`（`HTTPTools`）、`web_search`、`todo_write`；`NetPolicy` / `CheckURL` / `IsBlockedIP` / `GuardDialContext`（SSRF 防护）；`Exec` 统一执行包装（参数解码 → 失败信封 → 脱敏截断）与 `ArgString(s)` 宽松取参；`Toolset` 按名字装配。 |
| `plan/` | 通用 Plan 能力：`plan` / `explore` / `reflect` 三个工具（`Tools`）、`Planner`（强制模型以 `plan` 工具调用输出决策）、`Validate`（首错返回）、`Store`（`MemoryStore` / `FileStore`，planning-with-files 布局）。只记录目标、分工、确认与进度，不执行任何操作。 |
| `group/` | 群聊引擎：栅栏（规则短路 + Planner）→ 人工确认 → 依赖调度 → 事件广播；状态机、registry、broker、`RunHeadless`、`Converge`。宿主实现 `Store` / `Runner` / `Members` 三个端口。 |
| `cloudagent/` | 单助手运行循环 `Runtime`（`RunProvider`、`RunPreparation`、事件、线程占用、超时与取消覆盖准备期）、`AwaitConfirmation`（工具级确认）、`ToolCallGate`、`ProcessRunner`（argv 子进程 + 有界输出）、MCP 工具装配（`MCPPolicy` fail-closed）、`AssembleInstruction`。 |
| `mcp/` | MCP HTTP / stdio / in-memory client、in-memory server、分页上限、`HeaderTransport`。 |
| `skills/` | 从任意 `fs.FS` 加载 Markdown 技能文档（`ReadDocument` 去 frontmatter）；不内置任何平台技能或命令白名单。 |
| `cli/` | 本地命令行：REPL、一次性运行、JSONL 会话、压缩、MCP 配置、规划工具组接线。 |
| `cmd/` | CLI 入口，只依赖模块根包的 `RunCLI` / `SetCLIVersion` 门面。 |
| `docs/` | 设计笔记（Eino / Pi 调研），不是 API 契约。 |

依赖方向：`tools → agentcore, mcp`；`plan → agentcore`；`group → agentcore, plan`；`cloudagent → agentcore, mcp, tools`；`cli → agentcore, mcp, plan, tools`。`cloudagent` 与 `cli` 互不 import，`plan` / `group` / `tools` 不得 import `cloudagent` / `cli`，任何包不得 import 根模块。

SimpleBase 专属能力全部位于 `internal/agentbridge`：项目与身份、数据库/KV/S3/日志/沙盒工具、SimpleBase CLI 与委托令牌、模块目录、技能文档和命令授权。API 层只负责 HTTP 与持久化，并以 `internal/api/agent_plan_tools.go` 把 `plan.Tools` 接到每一次云助手 run（群任务带群约束，单助手只做结构校验）。

## 接入方式

`cloudagent.Runtime` 只依赖 `ChatClient` 和可选的 `RunProvider`：

```go
rt := &cloudagent.Runtime{
    LLM: client,
    Provider: cloudagent.RunProviderFunc(func(ctx context.Context, req cloudagent.RunRequest, emit func(cloudagent.Event)) (cloudagent.RunPreparation, error) {
        return cloudagent.RunPreparation{
            Model: "model-name", ModelScope: "tenant-or-workspace",
            Instruction: "You are a helpful assistant.",
            Tools: toolsFor(req.Context), // 例如 (&plan.Tools{Store: &plan.FileStore{Dir: dir}}).All()
            Cleanup: func() {},
        }, nil
    }),
}
```

`RunRequest.Context` 是 provider 与 MCP connector 共有的 opaque context，核心 Runtime 不解释它；工具经 `cloudagent.RunContextFrom(ctx)` 取到 `RunID` / `ThreadID` / `Emit`。`RunPreparation` 统一承载本轮模型、提示词、工具、清理函数和工具调用回调。无 provider 时可直接使用 `Runtime.Instruction`、`Runtime.Tools` 与 `Agent` 字段。

### 规划工具组（`plan.Tools`）

- `plan`：`action=create|update|replan|complete`。单助手场景只需 `goal` + `steps` + `progress`（无 `Constraints` 时只做结构校验，`dispatches` 可空）；群场景由 `Constraints` 强制成员集合、被 @ 的助手必须出现、human 可用性与任务图上限，`replan` 必须带 `dispatches`，`update` 只改进度类字段。
- `explore`：追加一条探索记录（goal / questions / reads / findings / unknowns）到 `explore.jsonl`，返回累计摘要；**不读文件**，证据由 `read` / `glob` / `grep` 先取。
- `reflect`：一次无工具的模型调用产出 `{aligned, risks, counterpoints, next_actions, facts}`，facts 写入 `facts.md`；`LLM` 为 nil 时退化为确定性检查清单。由模型自主决定调用。
- `OnDecision(ctx, d)` 在 `plan` 记录决策后回调，宿主据此广播 `plan` 帧；`ReplayCall` 从已落库的工具卡片重放 `plan` 调用，恢复「当前计划」供后续 run `update`。
- `FileStore` 布局：`plan.md` / `progress.md` / `findings.md` / `facts.md` / `explore.jsonl` / `decision.json`，临时文件 + rename 写入，单文件默认 256 KiB 超限滚动。本地 CLI 的计划目录在 `<sessions>/<会话ID>/plan/`。

### 群聊引擎（`group.Engine`）

`NewEngine(planner, runner, store, members)` 后：

- `Submit(ctx, TurnInput)`：规则短路（单成员 / 无模型走 `fallback_agent`）或 Planner 决策 → `plan.ApplyConfirmPolicy`（模型建议 ∪ 宿主规则，`destructive` 永远强制）→ 需要确认停在 `awaiting_plan_confirm`，否则后台执行。执行循环以 `context.WithoutCancel` + 轮次级 cancel 运行，SSE 只是订阅者。
- `ConfirmPlan`（批准 / 编辑后批准 / 拒绝）、`Cancel`、`RetryTask`、`ReplyHuman`、`Resume`、`RunHeadless`（cron：需要确认按策略拒绝并记录）、`Converge`（重启后把非活动轮次收敛为 `interrupted`）、`Subscribe`（按 `seq` 续接）。
- 轮次状态：`planning → awaiting_plan_confirm → running ⇄ waiting_human → completed | failed | canceled | interrupted`，`planning → rejected`；所有迁移经 `Store.Transition*`，终态不被覆盖。
- SSE v3 帧：`turn` / `plan` / `plan_confirmation` / `progress` / `task` / `human` / `confirmation_required` / `confirmation_resolved` / `end`，结构帧落事件表且实时与回放同形，`seq` 原子单调。

## MCP 安全

`MCPConnector` 按 `RunRequest` 建立本轮会话；凭据、租户授权和网络出口策略由接入方实现。`MCPPolicy` 对工具做只读/破坏性分类：策略缺失或出错时 fail-closed；不可写 Agent 不能调用写工具；破坏性调用需要 `AwaitConfirmation`，无人值守运行直接拒绝。会话清理顺序为 stop → MCP 会话集 Close（集合内按注册正序、全部尝试）→ `RunPreparation.Cleanup` → 释放线程。

本机 `tools` 的 shell 开启隔离但环境不支持时返回错误，不退回裸执行。子进程不继承 `GOCLOUDAGENT_*` 或名称含 TOKEN / SECRET / PASSWORD / API_KEY 等凭据变量。`http` / `web_fetch` 默认拒绝内网与链路本地地址（`NetPolicy`），CLI 需 `http_allow_private=true` 显式开启。

## CLI

```
gocloudagent                        interactive REPL (needs a TTY)
gocloudagent -p "prompt"            one-shot headless run（--json 输出 JSONL 事件）
gocloudagent run [flags] "prompt"   headless with flags（--cwd / --yolo / --shell-isolation）
gocloudagent resume <id> [prompt]   continue an existing session
gocloudagent sessions|config|mcp    manage sessions, ~/.gocloudagent/config.json and MCP servers
gocloudagent group list|show|new    本地群：成员 = 模型 + 指令 + 工具子集
gocloudagent group run <name> "msg" [--yes]   headless 跑一轮群消息（Plan 栅栏 → 分工 → 结果）
```

配置文件 `~/.gocloudagent/config.json`（权限 0600）：

```json
{
  "endpoint": "https://api.openai.com/v1",
  "token": "...",
  "model": "...",
  "http_allow_private": false,
  "mcp_servers": {
    "fs": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "."],
      "read_only_tools": ["read_file", "list_directory"]
    }
  }
}
```

`endpoint` 是完整的 OpenAI 兼容基础地址，客户端只追加 `/chat/completions` 或 `/models`，不拼接平台业务路径（接入 SimpleBase 时由调用方配置完整地址，例如 `http://localhost:8080/v1/projects/<project-id>/v1`）。配置只读取 `GOCLOUDAGENT_*` 环境变量，不读取 `SIMPLEBASE_*`，也没有 `project_id` 字段。HTTP MCP 的 Header 值支持 `${VAR}` 展开（配置里保留占位符）。REPL 的工具集由 `tools.Toolset` 组装，`plan` / `explore` / `reflect` 经 `Extra` 追加并绑定到会话的 `FileStore`。

### 本地群模式（`group`）

`~/.gocloudagent/groups/<name>.json` 定义一个群；成员 = 模型 + 指令 + 工具子集（`tools` 为空表示全部已装配工具）。群里的每条消息都先经 `group.Engine` 的 Plan 栅栏：单成员且未 @ 时规则短路直发，否则由 `plan.Planner` 以 `plan` 工具产出分工，再按依赖顺序把每个任务交给对应成员（每个任务是一个独立的 `SessionRunner` 会话，带 `group` / `task` 元数据，可单独 `resume`）。

```json
{
  "name": "review",
  "routing_prompt": "先摸清结构再动笔；需要多步时按依赖顺序分工。",
  "fallback_member": "explorer",
  "confirm_policy": "auto",
  "members": [
    { "id": "explorer", "instruction": "You map the code before anyone changes it.", "tools": ["read", "glob", "grep"] },
    { "id": "writer",   "instruction": "You implement exactly what the plan assigns.", "tools": ["read", "write", "edit", "bash"] }
  ]
}
```

- `gocloudagent group new <name>` 生成单成员样例（不覆盖已有文件）；`group show <name>` 查看成员与策略；`group run <name> "消息" [--yes] [--json]` headless 跑一轮。`--yes` 批准需要确认的计划，否则 headless 按策略拒绝并记录；人工任务只提示，到 REPL 里处理。
- REPL：`/group` 列出群，`/group <name>` 进入（之后的输入都是群消息，`@成员` 是硬约束，`@me` 把任务交给人），`/group status` 看最近一轮的计划与任务，`/group resume` 恢复中断 / 等人的轮次，`/group off` 回到单助手会话。整份计划需要确认时终端提示 `[a]pprove / [e]dit / [r]eject`（`e` 可改某条指令或移除某个 ref）；人工待办在轮次停下时就地回复，或稍后 `/reply <说明>` / `/unable [说明]`。
- 落盘：计划文件在 `groups/<name>/plan/`（`plan.FileStore`：`plan.md` / `progress.md` / `decision.json`），轮次、任务与事件在 `groups/<name>/state.json`（临时文件 + rename，0600）。进程重启后残留的 `running` 轮次收敛为 `interrupted`，由 `/group resume` 显式恢复，不自动重跑。
- 成员任务里可以调用 `plan` 工具：`action=update` 记录自己步骤的进度；`action=replan` 让引擎回到栅栏（每轮次数受 `max_replans` 限制，默认 2）。

## 构建与测试

```bash
cd gocloudagent
GOWORK=off go build ./...
GOWORK=off go vet ./...
GOWORK=off go test -race ./...          # -short 跳过创建 2 万文件与大输出用例
GOWORK=off go list -deps ./... | grep -c 'SimpleBase/internal'   # 期望 0
```
