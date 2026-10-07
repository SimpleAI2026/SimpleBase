---
title: 云沙盒概览
order: 1
---

# 云沙盒

云沙盒是项目级隔离 Linux 环境。控制台「自动化 → 云沙盒」、HTTP API、SDK 和云 Agent 使用同一个资源管理器；命令与文件只在 Cloud VM 内执行，不读写 SimpleBase 用户库 / S3 / 系统库。默认网络无出站权限。

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

密钥只能用 `SIMPLEBASE_SANDBOX_API_KEY` 环境变量提供（生产环境 YAML 里的密钥会被拒绝）。Cloud 不可用时功能关闭，绝不回退本地虚拟化。没有 Cloud key 的本地 e2e：`dev_mode: true` 并设置 `SIMPLEBASE_SANDBOX_BACKEND=fake`；fake 不启动真实 VM，只实现 `echo` / `cat` / `ls` / `pwd` / `sleep` / `exit` 等测试命令。

## 生命周期与轻量化

创建时只写一行元数据，首次运行命令、写文件或调用 `/start` 时才拉起 VM。调用结束后释放 SDK 句柄，但不主动保活。默认 1 核 / 256 MiB；Cloud 侧空闲 5 分钟后回收；单次 VM 最长运行 30 分钟。重新接回时文件是否仍在由 Cloud 生命周期决定；过期沙盒再次 `/start` 相当于重建工作区。按项目限制未删除沙盒数量，超出后返回 429。

执行接口只提供**同步 JSON 响应**，不提供 SSE / WebSocket 流。执行失败返回 HTTP 200 + 非零 `exit_code`，超时返回 200 + `timed_out=true`；VM 后端故障则返回 502。建议长任务将输出写入 `/workspace` 再读回文件。API 按 `database:read` / `database:write` 分权；系统项目不允许修改沙盒。

`Idempotency-Key` 仅在进程内内存中保存（24 小时、最多 1024 条），重启后失效；属于单实例尽力而为防重复提交，不落库。沙盒执行用量只记录不做配额拦截。

沙盒权限隔离边界是 Cloud microVM，不是 chroot：shell 可以读取 VM 镜像中的文件，但 VM 内不会注入 SimpleBase 凭据。管理文件接口仅接受 `/workspace` 下的绝对路径。

继续阅读：[API 全览](/docs/sandbox/api) · [e2e 用法](/docs/sandbox/e2e)。
