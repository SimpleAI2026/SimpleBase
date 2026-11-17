# gosandbox 出网隔离立项（v6.6）

> **状态（2026-11-17）：仅立项，未实施。** 来源：`gosandbox-cleanup-plan.md` §B3，用户决定「单独立项」。同批 GSB-10 已把从未接入生产的 `DNSProxy` / `CheckAllowed` / `egress-allowlist` 整体移除，本文是移除之后「要不要、怎么重做出网拦截」的起点。

## 1. 问题（读代码得出，未在 Linux 上复现）

`box/launch_linux.go` 只在 `none` 档加 `--unshare-net`。`egress` 档不隔离 netns，随后 `box/session.go` 让 pasta 去连 `bwrap` 进程的 netns——那就是**宿主自己的 netns**：

- 沙盒直接拥有宿主的全部网络：云元数据 `169.254.169.254`、私网 `10/8` `172.16/12` `192.168/16`、CGNAT `100.64/10`、宿主回环上的服务（含 SimpleBase 自己的监听）。
- `kiri-go-port-analysis.md` §6.5 / §8 要求「出网必须拦元数据、私网」，当前没有任何代码在执行。
- 这是典型的 SSRF 落点：沙盒内任意代码可读取云厂商临时凭证。

当前缓解：默认 `none`；`internal/config` 已校验 `sandbox.network` 只接受 `none` / `egress`；README 与 `config.example.yaml` 已写明 `egress` 不做过滤。**这是文案缓解，不是技术缓解。**

## 2. 先做的事：确认（Linux）

1. `sandbox.network: egress` 起一个沙盒，沙盒内 `ip route`、`cat /proc/net/route`，判断是否看到宿主路由。
2. 沙盒内访问宿主私网地址与（云上机器的）元数据地址，记录是否可达。
3. 在沙盒内访问宿主 `127.0.0.1:<服务端口>`，确认能否打到宿主进程。

三项结果决定是否升级为 P0。

## 3. 方案候选

| 方案 | 做法 | 优点 | 代价 / 风险 |
| --- | --- | --- | --- |
| A（推荐起点） | `egress` 档也 `--unshare-net`，由 pasta 在**沙盒自己的 netns** 里配网（`pasta --config-net --netns <pid>` 的本意）；pasta 自带 `--no-map-gw` / 地址映射选项，默认不把网关映射成宿主回环 | 隔离正确；复用现有 pasta | 需验证 pasta 版本；`box/session.go` 里的启动时序要保证 netns 已存在 |
| B | A + 在沙盒 netns 内用 nftables / 路由黑洞拦私网与元数据 | 地址层拦截，规则可审计 | rootless 下 netns 内配 nft 需要 `CAP_NET_ADMIN`（对自己的 userns 有效，但要在 agent 或 helper 里做），实现量中等 |
| C | 宿主侧（pasta 之外）做出站代理，仅放行白名单域名 | 能做域名级策略 | 对非 HTTP 协议无效；需要重新做白名单（GSB-10 刚删掉的就是这块的半成品） |

**建议顺序：A → B；C 只在有明确的域名白名单需求时再立项。**

## 4. 需求边界（需要用户确认）

- 沙盒之间是否需要互访（决定是否提供 `AllowPrivate` 这类开关）。
- 是否需要域名级白名单（决定要不要做 C）。
- 云元数据是否在任何场景下允许（默认不允许；若允许，必须是独立、显式的开关，不与「允许私网」混用）。
- 拦截命中时的可观测性：沙盒内看到 `ECONNREFUSED` / `ENETUNREACH`，宿主侧是否记 audit 事件（只记地址与规则名，不记载荷）。

## 5. 验收（实施时）

- Linux 真机用例：`egress` 档下，沙盒内访问元数据、私网、宿主回环**全部失败**；访问公网可达。
- `box.buildArgs` 参数层断言：任何档位都带 `--unshare-net`（`TestBuildArgsUnsharesNet` 的 egress 分支需随之改写，这是预期变更）。
- 不改 `go.mod`；pasta 缺失时 `egress` 档返回明确错误而不是退回共享宿主网络。

## 6. 风险与回滚

- 方案 A 会改变 `egress` 档的网络可达范围（收紧）。依赖「沙盒内访问宿主私网服务」的现有用法会失效——实施前需要用户盘点。
- 回滚：只回退该提交；`none` 档不受影响。

## 7. 执行记录（2026-11-17，已改代码）

用户决定「按 A → B 实施」，本文由立项转为已实施。

| 项 | 实际改动 |
| --- | --- |
| A（方案 A） | `box/launch_linux.go`：**任何档位**都加 `--unshare-net`（原先只有 `none` 档加）。pasta 改为在沙盒自己的 netns 里配网，不再借用宿主 netns。 |
| B（方案 B） | 新增 `agent/netfilter_linux.go`：用 netlink 往沙盒 netns 主路由表插 blackhole 路由，覆盖 `169.254.0.0/16`、`10/8`、`172.16/12`、`192.168/16`、`100.64/10`、`127/8`、`::1/128`、`fc00::/7`、`fe80::/10`。重复执行按 EEXIST 视为成功，地址族不可用时跳过。非 Linux 是空实现（`agent/netfilter_other.go`）。 |
| agent 档位 | `box` 把档位经 `--network none|egress` 传给 agent；`egress` 档装不上黑洞路由时 agent **退出码 3**（fail-closed），不会带着「以为隔离了、其实能读云凭据」的状态启动。 |
| capability | `box` 的 bwrap 参数在 `--cap-drop ALL` 之后加回 `CAP_NET_ADMIN`（只作用于沙盒 userns）：拉 lo 与加黑洞路由都要它。agent 配完调 `dropNetAdmin()` 清空 capability 集合，用户命令拿不到。 |
| pasta 接线 | `netx/pasta_linux.go`：`--netns` 从纯数字改为 `/proc/<pid>/ns/net`（纯数字会被当成 `/run/netns` 下的名字）；加 `--runas 0`（默认降权到 nobody 后打不开 netns、也没有 CAP_NET_ADMIN 建 tap）。以 root 运行时改为 `nsenter -t <pid> -n` 先进入沙盒 netns，再让 pasta 配 `/proc/self/ns/net`。 |
| pasta 时序 | `box/session.go`：pasta 从「launch 之后」移到「agent Hello 之后」（`startEgress`）。`cmd.Start` 返回时 bwrap 可能还没完成 unshare。supervise 模式下控制面没有 pid，egress 不可用并在日志里说明。 |
| 策略文件位置 | `host.InnerPolicyPath` 从 `/run/gosandbox/policy.json` 改为 `/tmp/gosandbox/policy.json`：沙盒根是 `--ro-bind / /`，bwrap 无法在只读根上创建 `/run/gosandbox`（`Can't create file ...: Read-only file system`）。`/tmp` 是 tmpfs，必然可写。 |
| 测试 | 新增 `box/egress_e2e_test.go`（4 个用例）；`TestBuildArgsUnsharesNet` 改为断言**所有档位**都 `--unshare-net`，并断言 `--network` 透传；`helperBin` 支持 `GOSANDBOX_HELPER_BIN` 预置二进制（验收机没有对应 Go 版本时也能跑真机用例）。 |
| 文案 | `gosandbox/README.md`、`internal/config/config.go`、`config.example.yaml`、`internal/sandbox/manager.go` 同步改为「独立 netns + 黑洞路由拦元数据/私网 + 不做域名白名单 + pasta 不可用时无出网（fail-closed）」。 |

### 真机验收（node1，TencentOS 4.4 / kernel 6.6.110，bubblewrap 0.12.0 + passt 20260716）

| 用例 | 结果 |
| --- | --- |
| `TestEgressBlocksMetadataAndPrivate` | **PASS**：`ip route show` 里确有 6 条 blackhole（元数据 / 私网 / CGNAT / 回环）。 |
| `TestEgressKeepsSandboxLoopback` | **PASS**：沙盒内 lo 是 UP。 |
| `TestEgressAllowsPublic` | **SKIP**：pasta 未能配网（见下），沙盒没有 default route，出网是关着的。 |
| `TestEgressHidesHostLoopback` | **SKIP**：同上，pasta 没起来时"连不上宿主回环"是空断言，不当作已验证。 |

**环境限制（不是代码缺陷）**：该内核上，从初始 userns 对 bwrap 创建的子 userns 做 `setns` 失败（`nsenter -U` 报 `reassociate to namespace 'ns/user' failed: Invalid argument`），pasta 因此无法在沙盒 netns 里建 tap（`TUNSETIFF ... Invalid argument`）。同一命令对 `unshare -n`（初始 userns 的 netns）是成功的，可见与 userns 后代关系有关。结果是 `egress` 档在这台机器上**没有出网**但仍完全隔离——符合 fail-closed，只是「公网可达」这一条没能验证，需在 pasta 可用的 Linux 环境补测。

`TestEgressAllowsPublic` / `TestEgressHidesHostLoopback` 用 `requireEgressConfigured` 区分「pasta 没配好（Skip）」与「配好了但隔离错（FAIL）」，避免 Skip 被读成通过。

### 顺带修掉的既有缺陷（真机第一次跑真 bwrap 时暴露）

这两条不在原计划里，但不修 egress 无法启动，也不属于行为变更：

1. `box`/`host` 的 `--dir /run/gosandbox` 在 `--ro-bind / /` 下必然失败（只读根建不了目录）。已改为 `/tmp/gosandbox`。
2. supervisor 重连后 `Exec` 用自增序号生成 id（`p1`），与 agent 里仍活着的旧进程撞名（`process p1 already exists`）；`Session.done` 在 reader / writer / 错误路径上会被 close 两次（`panic: close of closed channel`）。已改随机 id（`newObjectID`）与 `sync.Once`。
