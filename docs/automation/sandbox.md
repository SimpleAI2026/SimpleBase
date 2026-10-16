---
title: 云沙盒概览
order: 30
group: 云沙盒
---

# 云沙盒

云沙盒是项目级隔离 Linux 环境。控制台「自动化 → 云沙盒」、HTTP API、SDK 和云助手使用同一个资源管理器；命令与文件只在 Cloud VM 内执行，不读写 SimpleBase 用户库 / S3 / 系统库。默认网络无出站权限。

## 启用

```yaml
sandbox:
  enabled: true
  backend: cloud
  image: python:3.12-slim
  cpus: 1
  memory_mib: 256
  idle_timeout: 5m
  max_duration: 30m
  network: none
```

密钥只能用 `SIMPLEBASE_SANDBOX_API_KEY` 环境变量提供（生产环境 YAML 里的密钥会被拒绝）。Cloud 不可用时功能关闭，绝不回退本地虚拟化。没有 Cloud key 的本地 e2e：设置 `SIMPLEBASE_SANDBOX_BACKEND=fake` 且 `SIMPLEBASE_SANDBOX_ALLOW_FAKE=1`；fake 不启动真实 VM，只实现 `echo` / `cat` / `ls` / `pwd` / `sleep` / `exit` 等测试命令。

## 生命周期与轻量化

创建时只写一行元数据，首次运行命令、写文件或调用 `/start` 时才拉起 VM。调用结束后释放 SDK 句柄，但不主动保活。默认 1 核 / 256 MiB；Cloud 侧空闲 5 分钟后回收；单次 VM 最长运行 30 分钟。重新接回时文件是否仍在由 Cloud 生命周期决定；过期沙盒再次 `/start` 相当于重建工作区。按项目限制未删除沙盒数量，超出后返回 429。

执行接口只提供**同步 JSON 响应**，不提供 SSE / WebSocket 流。执行失败返回 HTTP 200 + 非零 `exit_code`，超时返回 200 + `timed_out=true`；VM 后端故障则返回 502。建议长任务将输出写入 `/workspace` 再读回文件。API 按 `database:read` / `database:write` 分权；系统项目不允许修改沙盒。

`Idempotency-Key` 仅在进程内内存中保存（24 小时、最多 1024 条），重启后失效；属于单实例尽力而为防重复提交，不落库。沙盒执行用量只记录不做配额拦截。

沙盒权限隔离边界是 Cloud microVM，不是 chroot：shell 可以读取 VM 镜像中的文件，但 VM 内不会注入 SimpleBase 凭据。管理文件接口仅接受 `/workspace` 下的绝对路径。

继续阅读：[API 全览](/docs/automation/sandbox-api) · [e2e 用法](/docs/automation/sandbox-e2e)。

## 与本机 shell 隔离的区别

云沙盒（本页）与 cloudagent CLI 的**本机 shell 隔离**是两套互不相干的机制：

| | 云沙盒 | 本机 shell 隔离 |
|---|---|---|
| 作用对象 | 服务端云助手的 `sandbox_shell`、HTTP API | 自托管机器上 cloudagent CLI 的 `shell` 工具 |
| 隔离边界 | Cloud microVM | 共享内核的进程沙盒：Linux 为 bubblewrap + Landlock + seccomp，macOS 为 `sandbox-exec` |
| 默认 | 随 `sandbox.enabled` | 关闭（`shell_isolation.enabled: false`） |
| 配置位置 | 服务器 `config.yaml` 的 `sandbox:` | CLI 的 `~/.cloudagent/config.json` 的 `shell_isolation`，或 `--shell-isolation <mode>` |

本机隔离**不是**云沙盒失败后的回退，云沙盒不可用时依然功能关闭。本机隔离不承诺挡住内核提权漏洞，多租户或不可信代码请使用云沙盒。`--yolo` 只跳过破坏性命令的确认，不会关闭本机隔离；隔离开启但环境不满足时，命令不会执行并返回 `isolation_unavailable`。详见 `gosandbox/README.md`。
