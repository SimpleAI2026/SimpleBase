# gocloudagent

SimpleBase 的云 Agent 运行时、本地命令行与本机工具集，独立 Go 模块 `github.com/linkxzhou/SimpleBase/gocloudagent`（根 `go.mod` 通过 `replace => ./gocloudagent` 引用）。**不得 import 根模块任何包**（含 `internal/...`）。

## 目录结构与依赖方向

```text
gocloudagent/
  cli_entry.go                                                   # 模块根包唯一的生产代码：RunCLI / SetCLIVersion 门面（cmd 只依赖它）
  cloudagent/                                                    # 云端 Agent 运行时（package cloudagent，宿主 internal/{api,app,agentbridge} 直接 import）
    run_types.go runtime.go runthread.go history.go stream.go    #   Runtime 装配、StartRun 主循环、线程占用 / 取消、历史与快照、流事件
    types.go ports.go ports_host.go                              #   无密钥 DTO（Principal/Agent/...）与宿主注入端口
    tools.go tools_sandbox.go                                    #   只读数据 / 日志工具、云沙盒工具
    skilltool.go callgate.go delegation.go cliexec.go            #   simplebase CLI 工具、确认闸门、委托令牌、CLIRunner
    mcptool.go mcpaccess.go                                      #   云端 MCP 工具与端口
    modules.go prompt.go errors.go trace.go
  agentcore/                                                     # 与运行时无关的模型协议、native→text 回退、流式合并、脱敏与事件抽象
  mcp/                                                           # MCP 客户端封装：Session 接口、命名、schema/结果转换、HTTP + stdio 传输
  cli/                                                           # 本地命令行外壳：agent_loop / 命令 / JSONL 会话 / 压缩 / REPL / mcp_*（本地 MCP 配置与适配）
  tools/                                                         # 本机工具：shell（可选经 gosandbox/host 隔离）、读写编辑文件、glob、grep
  skills/                                                        # 内嵌 SimpleBase 技能文档（go:embed）+ 前缀白名单 / 加载 / 对账解析
  cmd/                                                           # 本地 CLI 二进制入口（只 import 模块根包，产物输出到 build/bin/gocloudagent）
  docs/                                                          # eino-guide.md / pi-guide.md（参考文档）
  bench/                                                         # Agent 评测与冒烟脚本（Python，不进 go.mod）
```

- 模块内部单向依赖：`cloudagent` → `agentcore` / `mcp` / `skills`；`cli` → `agentcore` / `tools` / `mcp`；`cloudagent` 与 `cli` **互不 import**；`skills` / `agentcore` 不得 import 上层。
- 根包不再导出业务符号：宿主一律 `import ".../gocloudagent/cloudagent"`；技能前缀 / 加载 API 在 `skills`（`skills.AllowArgv` / `Prompt` / `Body` / `Known` / `IDs` / `Prefixes` / `EmbeddedAllows` / `EmbeddedReadOnly`），`module → skill` 的映射 `SkillsForModule` 留在 `cloudagent`。
- 跨模块唯一允许依赖为 `github.com/linkxzhou/SimpleBase/gosandbox/host`（供本机 `shell` 隔离执行），不得直接 import `gosandbox/vm` 或根模块。
- 宿主的身份转换、`CommandPolicy`（只读/破坏性判定与脱敏）、委托令牌签发、`InProcessCLI` 与错误分类全部由宿主侧 `internal/agentbridge` 适配注入。
- 安全边界：`Runtime.Policy` 缺失或 `Classify` 报错时必须 fail-closed（返回 `command_policy_unavailable`，不确认、不执行）；本机 `shell` 开启隔离但环境不支持时返回错误，绝不退回裸执行。
- MCP（planv6.2）：`mcp/` 只暴露 `Session` / `ToolInfo` / `CallResult`，SDK 类型不出该包；`mcp.HTTPOptions.HTTPClient` 传 nil 直接报错（SDK 不做 SSRF 防护）。云端凭据与出口策略由宿主 `internal/agentbridge` 注入；CLI 的 stdio 子进程不继承父进程环境，`shell_isolation` 开启时 stdio 一律拒绝。

## MCP 配置（CLI）

`~/.gocloudagent/config.json`：

```json
{
  "mcp_servers": {
    "github": { "url": "https://example.com/mcp",
                "headers": {"Authorization": "Bearer ${GITHUB_TOKEN}"} },
    "fs":     { "command": "npx", "args": ["-y", "@modelcontextprotocol/server-filesystem", "."],
                "read_only_tools": ["read_file", "list_directory"] }
  }
}
```

`gocloudagent mcp list` 看配置与启用状态（URL query 与 header 值均掩码），`gocloudagent mcp tools [server]` 连接并列出工具，REPL 里 `/mcp` 显示已装配的工具。非 `read_only_tools` 白名单内的工具首次调用需确认，`--yolo` 跳过确认。

## 构建与测试

```bash
cd gocloudagent
GOWORK=off go build ./...
GOWORK=off go vet ./...
GOWORK=off go test -race ./...
```
