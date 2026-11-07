# 可选：云沙盒（E2B 兼容 API）

沙盒是**可选能力**：需要实例所在机器具备本机进程隔离条件（Linux `bubblewrap` + Landlock，
macOS `/usr/bin/sandbox-exec`）。条件不满足时服务照常运行、沙盒 `available=false`，
本用例整体打印 `SKIP` 并退出 0，不计 FAIL。

能力可用时走一遍控制面：`capabilities → create → get → list → pause → delete`，
并确认删除后列表里不再出现该沙盒。

## 前置

- 服务端已启动，`SIMPLEBASE_BIN` / `SIMPLEBASE_URL` / `SIMPLEBASE_TOKEN` / `SIMPLEBASE_PROJECT_ID` 已导出。
- `simplebase sandbox capabilities` 返回 `available=true`；否则 SKIP。
- token 需要 `database:read` / `database:write`；实例必须 `writable=true`。
- 本机（macOS，DSH 沙箱内）`sandbox-exec` 返回 `Operation not permitted`，因此这里必然是 SKIP；
  在具备隔离能力的 Linux 主机上才执行完整流程。

## 运行

```bash
export CASE=14-sandbox
./examples/capabilities/14-sandbox/run.sh
```

## SKIP 判定

- `sandbox capabilities` 返回 `available=false`（含 `reason` / `backend`）→ SKIP，jsonl 写一条
  `ok=true, detail="skipped: 云沙盒不可用（...）"` 的记录。
- 接口返回 `sandbox_unavailable` / `not_found` / `forbidden` → SKIP。
- 其他异常（例如 500）按 FAIL 处理。

## 命令与断言

```bash
simplebase sandbox capabilities            # available=true, backend 非空
simplebase sandbox create --name cap14-<suffix> --timeout 60
simplebase sandbox get --id "$SID"
simplebase sandbox list
simplebase sandbox pause --id "$SID"
simplebase sandbox delete --id "$SID"
```

- create 返回 `sandboxID` 非空，`state` 或 `x_simplebase.status` 非空；
- get 的 `sandboxID` 与创建一致；list 里恰好 1 个；
- pause / delete 返回 `ok=true`；删除后 list 为 0 个。

## 已知边界

- 沙盒执行后端是进程隔离（共享内核），不是虚拟机；本机隔离不可用时绝不退回裸执行。
- 网络恒为关闭；cgroup 资源限制只在 cgroup v2 已委派时生效。
- 本用例不创建数据库；只创建一个沙盒并在结尾删除。
