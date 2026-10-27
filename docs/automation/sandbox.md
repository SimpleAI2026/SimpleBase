---
title: 云沙盒概览
order: 30
group: 云沙盒
---

# 云沙盒

云沙盒是项目级隔离命令环境。控制台「自动化 → 云沙盒」、HTTP API、SDK 和云助手使用同一个资源管理器；命令与文件只在各自的工作区内执行，不读写 SimpleBase 用户库 / S3 / 系统库。网络恒为关闭。

后端是**本机进程隔离**（复用 `gosandbox/host`），不是虚拟机，也不依赖任何外部沙盒服务，**没有 API Key**。

## 启用

```yaml
sandbox:
  enabled: true
  backend: local
  cpus: 1
  memory_mib: 256
  idle_timeout: 5m
  max_duration: 30m
```

完整字段见 `config.example.yaml`。

| 平台 | 隔离方式 | 前置条件 |
|---|---|---|
| Linux | bubblewrap + Landlock + seccomp | bubblewrap >= 0.8.0（非 setuid）、内核 Landlock ABI >= 3、可用的用户命名空间、与服务同目录的 `gosandbox-inner`（`./build.sh` 会构建） |
| macOS | 系统自带 `/usr/bin/sandbox-exec`（Seatbelt） | 无 |

启动时会做一次真实的隔离冒烟测试。本机不满足条件时，服务照常启动，仅云沙盒功能被跳过，并在日志里写明原因；**绝不退回无隔离执行**。

没有隔离环境的本地 e2e：设置 `SIMPLEBASE_SANDBOX_BACKEND=fake` 且 `SIMPLEBASE_SANDBOX_ALLOW_FAKE=1`；fake 是内存假后端，只实现 `echo` / `cat` / `ls` / `pwd` / `sleep` / `exit` 等测试命令。

## 隔离边界与限制

这是共享内核的进程沙盒，**不是 VM**，只适合自托管 / 受信任用户，不适合多租户不可信代码，也不承诺挡住内核提权漏洞。

- 工作区可写，其余文件系统只读；`storage.data_dir`、`database.cache_dir`、`config.yaml`、`.env` 和其他沙盒的工作区对命令不可见。
- 命令的环境变量不继承服务进程，只有最小集合加请求里传入的 `env`（拒绝 `SIMPLEBASE_`、`AWS_` 前缀）。
- 网络恒为关闭，请求 `network: public` 会被拒绝。
- `cpus` / `memory_mib` / `pids_max` 仅在 cgroup v2 已委派给服务进程时才生效，否则**不限制**（启动日志会提示）；macOS 恒不限制。
- 工作区内 `.gocloudagent` 目录只读；`.git/hooks`、`.git/config`、`.git/info/attributes`、`.gitmodules`、`.mcp.json`、`.vscode`、`.idea` 在**已存在**时也只读（它们会被宿主侧的 git、MCP 客户端、IDE 执行或读取）。`.git` 的其余部分可写，所以沙盒内 `git init` / `add` / `commit` 可用。工作区里暂无 `.git` 时不做保护（否则 `git init` 会失败），`git init` 之后的下一条命令起 `.git/hooks` 即受保护；同一条命令里「`git init` 后写入 hook」的窗口无法关闭。`HEAD`、`refs`、`objects` 可写，沙盒内命令能改写分支指向。
- 镜像字段只是展示标签，不会拉取或运行任何镜像；命令使用的是宿主机上的工具链（如宿主有 `python3` 才能跑 `python3`）。

## 路径约定

API 层的文件路径仍是 `/workspace` 下的绝对路径（由 `sandbox.workdir` 决定）。但**命令内部看不到字面量 `/workspace`**：命令的当前目录就是该沙盒的真实工作区，请在命令里使用相对路径。例如先用 `PUT .../files` 写入 `/workspace/a.py`，再执行 `python3 a.py`，而不是 `python3 /workspace/a.py`。

## 生命周期与轻量化

创建时只写一行元数据，首次运行命令、写文件或调用 `/start` 时才创建工作区目录。没有常驻进程：`/stop` 只是标记，文件保留；删除沙盒会删除整个工作区。默认 1 核 / 256 MiB；空闲 5 分钟后由 reaper 标记过期；单个沙盒最长 30 分钟。过期沙盒再次 `/start` 相当于重建工作区。按项目限制未删除沙盒数量，超出后返回 429。

执行接口只提供**同步 JSON 响应**，不提供 SSE / WebSocket 流。stdout 与 stderr 合并后放在 `stdout` 字段（`stderr` 恒为空）。执行失败返回 HTTP 200 + 非零 `exit_code`，超时返回 200 + `timed_out=true`；隔离不可用等后端故障返回 502。建议长任务将输出写入工作区再读回文件。API 按 `database:read` / `database:write` 分权；系统项目不允许修改沙盒。

`Idempotency-Key` 仅在进程内内存中保存（24 小时、最多 1024 条），重启后失效；属于单实例尽力而为防重复提交，不落库。沙盒执行用量只记录不做配额拦截。

继续阅读：[API 全览](/docs/automation/sandbox-api) · [e2e 用法](/docs/automation/sandbox-e2e)。

## 与 CLI 本机 shell 隔离的关系

云沙盒（本页）与 gocloudagent CLI 的**本机 shell 隔离**共用同一套底层（`gosandbox/host`），但配置与用途互相独立：

| | 云沙盒 | CLI 本机 shell 隔离 |
|---|---|---|
| 作用对象 | 服务端云助手的 `sandbox_*` 工具、HTTP API | 自托管机器上 gocloudagent CLI 的 `shell` 工具 |
| 默认 | 随 `sandbox.enabled` | 关闭（`shell_isolation.enabled: false`） |
| 配置位置 | 服务器 `config.yaml` 的 `sandbox:` | CLI 的 `~/.gocloudagent/config.json` 的 `shell_isolation`，或 `--shell-isolation <mode>` |

两者都不承诺挡住内核提权漏洞。`--yolo` 只跳过破坏性命令的确认，不会关闭本机隔离；隔离开启但环境不满足时，命令不会执行。详见 `gosandbox/README.md`。
