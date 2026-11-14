# gocloudagent

SimpleBase 的云 Agent 运行时、本地命令行与本机工具集，独立 Go 模块 `github.com/linkxzhou/SimpleBase/gocloudagent`（根 `go.mod` 通过 `replace => ./gocloudagent` 引用）。**不得 import 根模块任何包**（含 `internal/...`）。

## 目录结构与依赖方向

```text
gocloudagent/
  runtime.go  tools.go  cli_tool.go  modules.go  skills.go ...   # 云端 Agent 运行时、只读/沙盒/CLI 工具封装与门面（RunCLI / SetCLIVersion）
  agentcore/                                                     # 与运行时无关的模型协议、native→text 回退、流式合并、脱敏与事件抽象
  cli/                                                           # 本地命令行外壳：命令解析、JSONL 会话、上下文压缩、REPL
  tools/                                                         # 本机工具：shell（可选经 gosandbox/host 隔离）、读写编辑文件、glob、grep
  skills/                                                        # 内嵌 SimpleBase 技能文档（go:embed）
  cmd/gocloudagent/                                              # 本地 CLI 二进制入口（只 import 模块根包门面，产物输出到 build/bin/gocloudagent）
  bench/                                                         # Agent 评测与冒烟脚本
```

- 模块内部单向依赖：`cli` / 根包 → `agentcore`、`tools`；`agentcore` 不得反向 import `gocloudagent`。
- 跨模块唯一允许依赖为 `github.com/linkxzhou/SimpleBase/gosandbox/host`（供本机 `shell` 隔离执行），不得直接 import `gosandbox/vm` 或根模块。
- 宿主的身份转换、`CommandPolicy`（只读/破坏性判定与脱敏）、委托令牌签发、`InProcessCLI` 与错误分类全部由宿主侧 `internal/agentbridge` 适配注入。
- 安全边界：`Runtime.Policy` 缺失或 `Classify` 报错时必须 fail-closed（返回 `command_policy_unavailable`，不确认、不执行）；本机 `shell` 开启隔离但环境不支持时返回错误，绝不退回裸执行。

## 构建与测试

```bash
cd gocloudagent
GOWORK=off go build ./...
GOWORK=off go vet ./...
GOWORK=off go test -race ./...
```
