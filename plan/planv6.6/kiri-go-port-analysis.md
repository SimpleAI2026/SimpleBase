# kiri 架构剖析与 gosandbox 轻量容器平台（gobox）方案

> 对象：`thirdparty/kiri`（Rust，Apache-2.0，v0.2.x）与 `gosandbox`（Go，本仓独立模块）
> 目的：第一部分把 kiri 拆到「文件 → 函数」粒度；第二部分以容器运行时的视角，给出在 gosandbox 之上构建**轻量容器管理平台**的方案与能力补齐清单。
> 结论先行：kiri 是**容器运行时**（Docker Engine API 兼容），gosandbox 是**单命令沙盒**。目标不是移植 kiri，而是借它的 6 个核心机制（agent 当 PID 1、帧协议、层解包防护、seccomp 默认表、`CLONE_INTO_CGROUP`、日志格式），把 gosandbox 升级为「rootless、每沙盒一个常驻 agent」的轻量容器，对外仍是现有 E2B API。
> 修订：2026-11-17 第二版，删除「Go 版 kiri 移植路线」（原 §4-§7），改为 gobox 方案（§4-§13）。

---

## 0. 一句话定位

| | kiri | gosandbox |
|---|---|---|
| 是什么 | 轻量 Docker Desktop 替代品：一个 VM + 一个 PID 1 agent + Docker Engine API | 一条命令的 OS 级隔离执行器 |
| 隔离边界 | **虚拟机**（Apple Virtualization.framework / WSL2 / Linux 宿主内核） | **共享内核**（bwrap + Landlock + seccomp / Seatbelt） |
| 对外接口 | `unix://~/.kiri/run/docker.sock`（Docker API 1.47） | Go 包 API：`host.Run` / `host.Start` / `vm.Driver` |
| 生命周期 | 长驻 daemon，容器可 `-d`、可重启、可休眠唤醒 | 一次性命令 / 一个会话进程 |
| 镜像 | 完整 OCI 拉取、解包、overlayfs 分层、push | 无 |
| 网络 | bridge/veth/nftables/DNAT/内嵌 DNS/端口发布 | 恒关闭 |
| 代码量 | ~7 万行 Rust（含 UI/Windows） | ~6 千行 Go |

**关键判断**：gosandbox 的 `host` 包 ≈ kiri 的 `kiri-init/linux/container.rs` 的**一个子集**（只做「进命名空间 + 挂载 + 降权 + exec」），而且用的是 bwrap 而不是自己 clone3。kiri 的其余 90% 在 gosandbox 里完全没有对应物。

---

## 1. 总体架构

```
┌──────────────────────────────────────────────────────────────────────┐
│ 客户端                                                                │
│  docker CLI / compose / buildx / kiri CLI / kiri UI / kubectl        │
└───────────────────────────┬──────────────────────────────────────────┘
                            │ HTTP over unix socket (Docker Engine API 1.47)
                            │ 或 npipe://./pipe/kiri (Windows)
┌───────────────────────────▼──────────────────────────────────────────┐
│ crates/kiri  ——  kiri daemon（宿主进程，非 root）                      │
│                                                                       │
│  daemon/mod.rs      监听 socket、信号、启动各子系统                     │
│  daemon/http.rs     手写 HTTP/1.1（keep-alive / chunked / upgrade）    │
│  daemon/api.rs      Docker API 路由 + JSON 编解码                       │
│  daemon/ops.rs      stats/top/pause/update/commit/export/prune/df/...  │
│  daemon/engine.rs   容器/exec/镜像的**唯一状态机**（containers.json）    │
│  daemon/vmlink.rs   到 guest 的**单条 vsock 连接**（多路复用 + 信用窗口）│
│  daemon/network.rs  网络与 IPAM（bridge/host/none + 用户网络 + IPv6）   │
│  daemon/ports.rs    端口发布（宿主 listener → DNAT / vsock relay）      │
│  daemon/volumes.rs  命名卷（volumes.json）                             │
│  daemon/logfile.rs  docker logs 落盘（Docker local 驱动格式）           │
│  daemon/builder.rs  托管 BuildKit 容器 + buildctl                      │
│  daemon/mirror.rs   把本地镜像暴露成只读 registry 给 BuildKit           │
│  daemon/dns.rs      *.kiri.local DNS（127.0.0.1:53）                    │
│  daemon/domains.rs  <name>.kiri.local 反向代理路由（80/443）            │
│  daemon/tls.rs      自建 CA（name constraints 限定 *.kiri.local）       │
│  daemon/nfs.rs      NFSv3 over unix socket → ~/Kiri（Finder 浏览）      │
│  daemon/health.rs   healthcheck 规则                                   │
│  daemon/hostopts.rs --ulimit/--sysctl/--device/--group-add 校验与下发   │
│  daemon/binds.rs    Windows 路径 / docker.sock 绑定源归一化             │
│  daemon/access.rs   Linux 宿主 socket 的 kiri 组 ACL + 每连接鉴权       │
│  daemon/usb.rs      USB 直通（宿主侧）                                  │
│  daemon/proxy.rs    代理设置（系统/自定义/无）                          │
│  daemon/pulls.rs    并发 pull 合并（同镜像只拉一次）                    │
│  daemon/filters.rs  Docker filters 查询参数                            │
│  daemon/names.rs    随机容器名 adjective_scientist                     │
│  daemon/time.rs     RFC3339（不引日期库）                              │
│  daemon/vmcpu.rs    VM CPU 占用                                        │
│  daemon/hostetc.rs  Mac 上给容器 bind 的 /etc/{passwd,group,...}       │
│  diag/*             日志、崩溃报告、脱敏                                │
└───────────────────────────┬──────────────────────────────────────────┘
                            │ vsock（VM）/ unix socket g<port>.sock（Linux 宿主）
┌───────────────────────────▼──────────────────────────────────────────┐
│ crates/kiri-init  ——  guest PID 1 + agent（~900KB 静态二进制）         │
│                                                                       │
│  linux/mod.rs       挂载内核文件系统、离开 initramfs、建数据目录        │
│  linux/agent.rs     **单线程 poll 事件循环**（核心）                    │
│  linux/container.rs clone3 进命名空间 + overlayfs + pivot_root + exec  │
│  linux/layers.rs    接收压缩层 → 解包到 layers/<hex>/（whiteout 处理）  │
│  linux/net.rs       uplink DHCP、bridge、veth、masquerade              │
│  linux/netlink.rs   手写 rtnetlink + nf_tables（不依赖 ip/nft 二进制）  │
│  linux/dns.rs       内嵌 DNS（127.0.0.11:53 + 各 bridge 网关）          │
│  linux/cgroup.rs    限额、freeze、stats、top                           │
│  linux/devices.rs   cgroup-v2 BPF 设备白名单                           │
│  linux/seccomp.rs   默认 syscall 过滤器（含 vsock 永久拒绝）            │
│  linux/hostopts.rs  ulimit/sysctl/device/group-add 落地                │
│  linux/inroot.rs    openat2(RESOLVE_IN_ROOT) 安全路径解析              │
│  linux/fsview.rs    容器文件系统只读视图（给 UI 浏览）                  │
│  linux/copy.rs      docker cp（fork 子进程，chroot 进容器）             │
│  linux/relay.rs     端口转发 guest 侧（vsock ↔ TCP/UDP）               │
│  linux/pid1.rs      --init（tini 语义）                                │
│  linux/volmount.rs  卷的 local 驱动（nfs/tmpfs/bind/cifs）             │
│  linux/usage.rs     docker system df 的 du 统计                        │
│  linux/usb.rs       USB/IP 设备侧                                      │
│  linux/transport.rs vsock / unix socket 抽象                           │
│  linux/sys.rs       syscall 薄封装（所有 unsafe 集中处）                │
└───────────────────────────┬──────────────────────────────────────────┘
                            │
┌───────────────────────────▼──────────────────────────────────────────┐
│ crates/kiri-vmm  ——  VM 抽象                                          │
│  apple.rs    Virtualization.framework（objc2，私有串行队列）           │
│  process.rs  Linux 宿主：agent 直接跑在宿主内核上，vsock 端口→unix sock │
│  stop.rs     停止/等待                                                 │
└──────────────────────────────────────────────────────────────────────┘

旁路 crate：
  kiri-proto       宿主↔guest 线协议（no_std，零分配）
  kiri-image       镜像引用、registry 客户端、blob store、tar 归档
  kiri-share-proto kiri share 隧道协议 + 构建签名
  kiri-win         Windows 侧（npipe、WSL 桥、WebView2 窗口、ConPTY）
```

---

## 2. 逐文件 / 逐函数职责

### 2.1 `crates/kiri-proto/src/lib.rs`（61KB，协议层）

**这是移植时第一个要抄的东西**，因为它定义了宿主与 guest 的全部契约。

帧格式：`len: u32 LE | kind: u16 LE | payload`，`len` 含 kind。`MAX_FRAME = 1 MiB`，`HEADER = 6`。

| 常量 | 值 | 含义 |
|---|---|---|
| `HOST_PORT` | 1024 | 宿主监听的 vsock 端口，guest 拨入 |
| `COPY_PORT` | 1028 | `docker cp` 专用端口，一次拷贝一条连接 |
| `DNS_RELAY_PORT` | 1037 | guest 解析失败时转发给宿主 |
| `USB_PORT` | 1040 | USB/IP 直通 |
| `PROTOCOL` | 4 | 线协议版本（3 加 `InputConsumed`，4 加 `UsageQuery`） |
| `MAX_OUTPUT_CHUNK` | 64 KiB | 单帧输出上限 |
| `INPUT_WINDOW` | 4 MiB | 宿主可超前发送的 stdin 字节数 |

`enum Msg`（43-181）—— 全部消息，按方向分组：

| 方向 | 消息 | 语义 |
|---|---|---|
| G→H | `Hello{protocol, init_start_ns, ready_ns, mem_total_kib, mem_available_kib}` | 连接后第一条，报启动耗时与内存 |
| 双向 | `Ping(u64)` / `Pong(u64)` | 保活 |
| H→G | `Shutdown` | flush 后关机 |
| G→H | `Error(&str)` | 人类可读失败 |
| H→G | `StatsRequest` → `Stats` | VM 资源占用 |
| H→G | `LayerQuery(digest)` → `LayerStatus(bool)` | 该层是否已解包 |
| H→G | `LayerBegin{LayerBegin}` → `LayerStored` | 后跟 `size` 字节裸 blob（**不在帧内**） |
| H→G | `Run(RunSpec)` → `Started{id,pid}` → `Output{id,stream,data}*` → `Exited` | 启动容器 |
| H→G | `Kill{id,signal}` / `Remove{id}` | 无回复 |
| H→G | `SetTime(u64)` | guest 无真实时钟，连接时下发 |
| H→G | `Input{id,data}` / `CloseStdin{id}` / `Resize{id,rows,cols}` | 无回复 |
| G→H | `InputConsumed{id,bytes}` | 主动上报，驱动信用窗口 |
| H→G | `Publish(Publish)` / `Unpublish{id}` | 内核 DNAT 端口发布 |
| H→G | `Exec(ExecSpec)` | 同 Run 的回复序列，用 `exec_id` |
| H→G | `Retain(StrList)` / `RetainLayers(StrList)` | 连接后清理孤儿 |
| H→G | `UsageQuery` → `Usage(StrList)` | `docker system df` |
| H→G | `FsStatRequest` / `FsListRequest` / `FsRead` → `FsStat`/`FsList`/`FsData` | 容器文件浏览 |
| G→H | `MemoryPressure` | PSI 触发，请求扩内存 |
| H→G | `RemoveVolume{name}` / `RemoveBridge{bridge}` | 无回复 |
| H→G | `Copy(CopySpec)` → `CopyStat`/`CopyDone` | `docker cp` |
| H→G | `Update{id,limits}` / `Freeze{id,frozen}` | `docker update` / `pause` |
| H→G | `StatsQuery{id}` → `ContainerStats` | `docker stats` |
| H→G | `TopQuery{id}` → `Top(StrList)` | `docker top` |
| H→G | `NetAttach` / `NetDetach` | 多网络接入 |
| H→G | `ListenQuery{id}` → `Listening(StrList)` | 容器监听的 TCP 端口 |
| H→G | `UsbNodesQuery` → `UsbNodes` | USB 设备节点位置 |
| H→G | `Route{dst,prefix,via,add}` | k8s pod/service 网段路由 |
| H→G | `UplinkQuery` → `Uplink{ip,gateway}` | NAT 网络租约 |
| 通用 | `Ack` | 成功 |

关键结构：`RunSpec`（436-459，含 id/layers/argv/env/cwd/user/hostname/flags/net/mounts/limits/caps/opts）、`ExecSpec`、`NetSpec`、`Limits{memory,cpu_quota,cpu_period,pids,shm_size}`、`ContainerStats`、`FsEntry`、`CopySpec`。

`run_flags`（350-378）位标志：`NET_HOST / NET_NONE / REMOVE / TTY / STDIN / PRIVILEGED / INIT / DNS_GATEWAY / NO_SECCOMP / NO_NEW_PRIVS / READONLY_ROOTFS`。

`net_opts`（399-433）字符串键值：`IP6 / GW6 / INTERNAL / NO_IPV4 / MAC / NETNS / DOMAINNAME / NAMESERVER`。

编解码：`Msg::encode`（840-1054）写入调用方缓冲；`decode`（1127-1295）借用切片；`Reader`（1062-1114）提供 `u8/u16/u32/u64/bytes/str/list/rest`。**零分配**是设计目标。

> **Go 移植要点**：Go 里不需要 `no_std`，但「帧 + 裸 blob 分离」和「信用窗口」必须保留，否则大文件传输会退化成内存拷贝。用 `encoding/binary` + `[]byte` 复用池即可。

### 2.2 `crates/kiri-image`（镜像层，宿主侧）

| 文件 | 职责 | 关键函数 |
|---|---|---|
| `lib.rs` | 导出面 | `Cancel / Reference / Client / Credentials / Platform / Progress / Store / Image / Layer / RunConfig` |
| `reference.rs` | 镜像引用解析 | `alpine`、`nginx:1.27`、`ghcr.io/o/r@sha256:…`；Docker Hub 别名归一 |
| `registry.rs`（64KB） | OCI/Docker registry 客户端 | `pull_with`（499-650，多架构选平台 + 逐层下载）、`manifest`、`download_with`（686-735，**断点续传**）、`token`（827-873，Bearer challenge）、`login`、`push`（956-1018）、`search`、`tags`、`parse_challenge`、`transient`（网络/5xx 可重试判定） |
| `connect.rs` | Happy Eyeballs（RFC 8305） | 250ms 轮换地址族，先连上者胜 |
| `store.rs` | 内容寻址 blob store | 布局 `blobs/sha256/<hex>` + `images.json` + `tmp/`；`blob_writer`（边下边校验）、`ingest`、`put_image`、`remove_image`、`set_pushed` |
| `archive.rs` | `docker load` / `docker save` | Docker 的 `manifest.json` 格式，流式写入 store |
| `tags.rs` | registry tag 列表分页 | `Link` header 翻页，版本号倒序 |
| `b64.rs` | Docker 风格 base64 | 写标准带 padding，读兼容 URL-safe / 无 padding |
| `cancel.rs` | 跨线程取消 pull | 关闭正在读的 registry 连接（含 mid-read） |

### 2.3 `crates/kiri-vmm`（VM 抽象）

| 文件 | 职责 |
|---|---|
| `lib.rs` | 统一导出：`apple::*`（macOS）或 `process::*`（Linux） |
| `apple.rs`（29KB） | Virtualization.framework：`Config`（CPU/内存/内核/initramfs/磁盘/virtiofs 共享/Rosetta）、`Vm::build`（456-566）、`VsockStream`、`Connector`（364-413，guest 拨入的监听）、`VmDelegate`/`PortDelegate`/`ListenerDelegate`（objc 回调）。**所有 VZ 对象只在一条私有串行队列上访问** |
| `process.rs` | Linux 宿主：agent 作为普通 root 进程跑在宿主内核，vsock 端口映射成 `KIRI_AGENT_DIR` 下的 `g<port>.sock`（agent 监听）/ `h<port>.sock`（daemon 监听） |
| `stop.rs` | 停止与等待（含强制停止超时） |

### 2.4 `crates/kiri-init`（guest，最核心）

#### `linux/mod.rs`（启动序列）

| 函数 | 职责 |
|---|---|
| `main`（34-45） | 记录 boottime，跑 `run`，失败打印后 `power_off` |
| `run`（47-69） | 分叉：Linux 宿主走 `host_dirs`；VM 走 `leave_initramfs` → `mount_kernel_filesystems` → `sethostname("kiri")` → `loopback_up` → `data_dirs` |
| `serve`（71-79） | `dial_host` → 发 `Hello` → `agent::serve` |
| `leave_initramfs`（84-104） | 把 PID 1 重挂到 tmpfs 上（内核拒绝在 initramfs 上 pivot_root），等价 `switch_root` |
| `mount_kernel_filesystems`（106-121） | proc/sysfs/devtmpfs/devpts/shm/cgroup2(`nsdelegate,memory_recursiveprot`)/run/tmp |
| `host_dirs`（136-163） | Linux 宿主：`/var/lib/kiri` 0710、子目录 0700、umask 022、清理上次残留 |
| `take_over`（174-215） | `agent.lock` flock 单实例；孤儿 agent 先 SIGTERM 再 SIGKILL |
| `host_cgroup` / `containers_cgroup` / `mount_cgroup2` / `delegated` | cgroup2 挂载与委派检测 |
| `data_dirs`（337-379） | 数据盘（ext4）挂载点、layers/containers/volumes 目录 |
| `rosetta`（385-404） | virtiofs 共享的 Rosetta 注册到 binfmt_misc |
| `dial_host`（406-422） | vsock 连 `HOST_PORT` |
| `meminfo` / `power_off` | /proc/meminfo 摘要；`reboot(RB_POWER_OFF)` |

#### `linux/agent.rs`（915 行，单线程事件循环）

`serve`（40-650）是**整个 guest 的心脏**：

1. 建 `signalfd`（SIGCHLD）、`term_signals`（Linux 宿主）
2. `net::uplink()` 拿 DHCP 租约（VM）或 `host_uplink()`（宿主）
3. 建各类监听：`relay::listen`（端口转发）、`listen_host_bound`（host.docker.internal）、`listen_framed`、`listen_ssh_agent`、`listen_engine`（容器内 docker.sock）、`copy::listen`、`usb::listen`、`listen_udp`
4. `memory_pressure_trigger`（PSI）
5. 主循环：每轮重建 `pollfd` 数组（宿主连接、signalfd、DHCP、DNS、各监听、每个进程的 stdout/stderr/stdin、每个 relay 的两端），`poll` 后分发

辅助函数：`flush_input`（702-730，把缓冲的 stdin 写进进程）、`held_kib`/`self_rss_kib`（气球）、`read_frame`、`owner`、`child_signals`、`stop_containers`、`reap`（868-890，回收子进程并生成 `Exited`）。

> **Go 移植要点**：Go 里不要照抄 poll 循环。用 goroutine + channel：每个容器一个 `outputPump` goroutine，每个 relay 一对 `io.Copy`，`os/exec` 的 `Wait` 天然回收。但**必须保留「单写者」约束**——所有帧写入走一个 `chan []byte` 到一个 writer goroutine，否则帧会交错。

#### `linux/container.rs`（1708 行，容器启动）

| 函数 | 职责 |
|---|---|
| `spawn`（221-402） | 完整启动流程：`mount_rootfs` → 建 cgroup → `apply_limits` → 解析 hostopts → `devices::restrict`（BPF）→ `prepare` → 写 /etc 文件 → `parse_mounts` → `stdio` → **`clone3`** → 父侧建 veth/DNS/sysctl → `finish_stdio` |
| `mount_rootfs`（406-437） | overlayfs：`lowerdir` = 各层（相对路径省字节）、`upperdir`/`workdir`/`rootfs` 在 `containers/<id>/` |
| `exec`（440-519） | `docker exec`：打开目标进程的 net/uts/ipc/cgroup/mnt ns，`setns(NEWPID)` 后 `clone3`，再恢复自己的 pid ns；容器自管 cgroup 时回退到 init 所在子 cgroup |
| `container_child`（1062-1122） | 子进程：`MS_REC|MS_PRIVATE` → 可选 `setns(NEWNET)` → `chdir(rootfs)` → `inroot::open_root` → `mount_container_filesystems` → 设备节点 → bind /etc → 应用 mounts → **`pivot_root(".",".")` + `umount2(".",MNT_DETACH)`** → 只读根 → hostname/domainname → loopback → 等网络就绪信号 |
| `finish_child`（1150-1278） | 共用收尾：stdio（PTY 走 `open_pty` + `send_fd` + `setsid` + `TIOCSCTTY`；管道走 `setsid` + dup2）→ `set_rlimits` → 逐 cap `PR_CAPBSET_DROP` → `PR_SET_NO_NEW_PRIVS` → **`seccomp::install`** → `resolve_user` + `setgroups/setgid/setuid` → `chdir(cwd)`（不存在则 mkdir -p）→ `find_executable` → 补 `HOME` → 恢复 SIGPIPE/SIGCHLD → `execve` |
| `mount_container_filesystems`（1284-1352） | /proc /sys /dev /dev/pts /dev/shm /dev/mqueue + 只读 cgroup 视图；privileged 则给 VM 的设备与可写 /sys |
| `copy_host_devices`（1359-1394） | `--device` 节点复制 |
| `harden_proc`（1402-1425） | /proc 的 hidepid 等加固 |
| `prepare`（681-722） | 补 PATH/HOSTNAME/TERM，默认 caps，`shm_size` 默认 64MiB |
| `write_etc_files`（726-782） | 生成 hosts/resolv.conf/hostname 到 `/run/kiri/<id>/`，子进程 bind 覆盖 |
| `resolv_conf`（816-843） | 用户网络 → `127.0.0.11`；否则 `--dns` 或 VM 的；`--dns-search`/`--dns-option` |
| `parse_mounts`（867-913） | 解析 `RunSpec.mounts` 的 `src:dst:opts` 行，按深度排序（父先子后） |
| `apply_mount`（947-987） | bind / tmpfs / volume / copy-up（新卷从镜像树填充） |
| `teardown` / `sweep` / `kill_cgroup` / `remove_cgroup` | 清理：杀 cgroup 内全部进程、删 overlay、删卷 |
| `resolve_user` / `lookup_gid_for_uid` / `find_executable` / `home_of` | 用户与可执行文件解析 |

#### `linux/layers.rs`（层解包）

`receive`（40-59）→ `install`（63-79，rename 原子出现）→ `unpack`（140-152）→ `untar`（157-194）。安全点：`escapes`（196-198）拒绝 `..` 逃逸；`whiteout_dir`（204-220）把 `.wh.` 前缀转成字符设备 0:0；`Capped`（124-138）限制解压膨胀比；`<hex>.done` 标记文件保证「目录出现即完整」。

#### `linux/net.rs` + `linux/netlink.rs`（网络）

`net.rs`：`uplink`（DHCP 拿 eth0）、`Dhcp`（283-421，完整 DHCP 客户端状态机）、`Bridges::ensure/bring_up/remove/update_v6`、`attach_container`（636-649，建 veth 对并塞进容器 ns）、`install_nat`/`refresh_nat`、`ipv6_forwarding`、`local_resolver`。

`netlink.rs`（48KB）：手写 `Msg` 构造器 + `Netlink` socket 封装。`create_bridge`、`create_veth`、`set_up`、`add_address`/`add_address6`、`add_default_route`/`add_default_route6`、`set_route`、`in_netns`（在目标 ns 里执行）、`nat_chain`、`redirect_rule`、`dns_redirect_rule`、`dnat_rule_from`、`published_masq_rule`、`forward_filter_in`、`localnet_guard`、`host_relay_guard`、`install_nat6`、`remove_nat`。

> **Go 移植要点**：**不要手写 netlink**。用 `vishvananda/netlink` + `google/nftables`（或 `netfilter/nftables`）。这是 kiri 为了「零依赖静态二进制」付出的代价，Go 侧没必要重复。

#### `linux/dns.rs`（37KB，内嵌 DNS）

`Dns`（95-110）持有多个 `Server`（每个 bridge 一个 + 127.0.0.11）。`serve`/`serve_in`（在容器 ns 内监听）、`register`/`register6`（名字→IP）、`set_upstream`、`ask`（512-525，转发）、`framed_reply`（vsock 转发给宿主）、`parse_question`（557-581）。

#### `linux/seccomp.rs`（默认过滤器）

`denied()`（92-146）是**移植时最该照抄的表**：

- 无条件下拒绝：`add_key`、`keyctl`、`request_key`、`syslog`、`userfaultfd`、`io_uring_setup/enter/register`
- 需 cap 才放行：`init_module`/`finit_module`/`delete_module`(CAP_SYS_MODULE)、`kexec_load`/`kexec_file_load`/`reboot`(CAP_SYS_BOOT)、`clock_settime`/`clock_adjtime`/`settimeofday`(CAP_SYS_TIME)、`acct`(CAP_SYS_PACCT)、`swapon`/`swapoff`/`quotactl`/`mount`/`umount2`/`pivot_root`/`open_tree`/`move_mount`/`fsopen`/`fsconfig`/`fsmount`/`fspick`/`mount_setattr`/`unshare`/`setns`/`name_to_handle_at`/`open_by_handle_at`/`lookup_dcookie`/`vhangup`(CAP_SYS_ADMIN)、`mbind`/`set_mempolicy`/`migrate_pages`/`move_pages`(CAP_SYS_NICE)、`bpf`(CAP_BPF)、`perf_event_open`(CAP_PERFMON)
- `deny_vsock`（176-182）：**无论什么 cap**，`socket(AF_VSOCK)` 一律 `EAFNOSUPPORT`（vsock 不做命名空间隔离，是通往宿主的后门）
- `boundary()`（187-195）：`--privileged` / `seccomp=unconfined` 时只保留 io_uring + vsock 两条

#### 其余 guest 模块

| 文件 | 职责 |
|---|---|
| `cgroup.rs` | `apply_limits`（memory.max/cpu.max/pids.max）、`freeze`（cgroup.freeze）、`stats`（memory.current/psi、cpu.stat）、`pids`、`top`（遍历 cgroup.procs 读 /proc） |
| `devices.rs` | `program`（77-117）生成 `BPF_PROG_TYPE_CGROUP_DEVICE` 指令；`restrict`（177-212）load + attach |
| `hostopts.rs` | `parse`（71-117）解析 opts 行；`resolve_devices`、`make_nodes`、`apply_sysctls`（在目标 ns 写 /proc/sys）、`set_rlimits`、`supplementary_groups` |
| `inroot.rs` | `open_root`/`open`（63-114）用 `openat2(RESOLVE_IN_ROOT)` 逐段解析，符号链接不能逃出容器根 |
| `fsview.rs` | `stat`/`list`/`read`，同样 `RESOLVE_IN_ROOT`；`MAX_ENTRIES=5000`、`MAX_READ=512KiB` |
| `copy.rs` | `accept`（23-51）→ fork 子进程 → `enter`（92-117，chroot 进容器）→ `copy_in`/`copy_out`/`layer_job`（未运行容器用私有挂载） |
| `relay.rs` | `Relay`（50-62）非阻塞双向泵；`listen_host_bound`（351-365）只接受宿主来的连接；`listen_ssh_agent`、`listen_engine` |
| `pid1.rs` | `run`（22-86）：`--init` 时容器 PID 1 是 agent 自己，程序作为子进程，信号转发、孤儿回收、退出码透传 |
| `volmount.rs` | `ensure`（27-75）按 `type=nfs/tmpfs/none/cifs` 挂载卷；`hide_password` 脱敏日志 |
| `usage.rs` | `measure`/`du`：`du -x` 语义（分配块、硬链接只算一次、不跨文件系统） |
| `usb.rs` | USB/IP 设备侧：`speed u32 | devid u32` 后接 USB/IP 协议 |
| `transport.rs` | `listen(port)`/`dial(port)`：VM 走 vsock，Linux 宿主走 `g<port>.sock`/`h<port>.sock` |
| `sys.rs` | 所有 syscall 薄封装：`mount`/`remount`/`umount_lazy`/`mknod`/`pipe`/`loopback_up`/`sethostname`/`clone3`/`child_fail`/`exec_fail`/`socketpair`/`send_fd`/`recv_fd`/`setns`/`open_pty`/`resize_pty` |### 2.5 `crates/kiri/src/daemon`（宿主侧，逐文件）

#### `mod.rs`（264 行，进程骨架）

| 函数 | 职责 |
|---|---|
| `state_dir` | `~/.kiri`（`KIRI_DATA_DIR` 可覆盖），**必须在 HOME 下** |
| `socket_path` | `~/.kiri/run/docker.sock`（`KIRI_SOCKET` 可覆盖） |
| `raise_open_files` | 软限制提到硬限制（macOS 上限 10240）；每条转发连接占 4-5 个 fd |
| `run`（81-215） | ① 阻塞 SIGINT/SIGTERM（**必须在起任何线程前**）② 已有实例则退出 ③ bind socket 0600 ④ `KIRI_SOCKET_GROUP` → chown 0660 + `access::Gate` ⑤ `Store::open` ⑥ `Engine::boot` ⑦ 崩溃报告发送器 ⑧ Finder NFS ⑨ `builder::watch` ⑩ `k8s::sync_at_start` ⑪ DNS + domains + TLS ⑫ `watch_own_app` ⑬ 信号线程 ⑭ accept 循环，每连接一线程 |
| `watch_own_app` | app 被删 → 交还 docker build 选择 → 关 VM → 退出 |
| `finder` | NFS 挂到 `~/Kiri`（`KIRI_NO_FINDER=1` 跳过） |
| `socket_group` | `getgrnam` + chown + 0660 |

#### `http.rs`（437 行，手写 HTTP/1.1）

`API_VERSION = "1.47"`、`MAX_BODY = 8 MiB`、`MAX_HEAD = 64 KiB`。

`Request`（20-28）：`method`、`path`（已剥 `/v1.xx` 前缀并 percent-decode）、`query: Vec<(String,String)>`、`headers`、`body`。方法：`query`、`queries`（重复键）、`flag`（`1`/`true`）、`header`（大小写不敏感）、`internal`（daemon 自调用）。

`read_request`（60-…）：请求行 + 头走 `take(MAX_HEAD)` 上限；`stream(method,path)` 为真时 body 不读，交给 `Body` 流式读。支持 `Content-Length`、`chunked`、`Upgrade`（attach 用）。

#### `api.rs`（3093 行，Docker API）

`serve`（22-50）→ `route`（92-310）。路由表（按代码顺序）：

| 方法 | 路径 | 处理 |
|---|---|---|
| GET/HEAD | `/_ping` | `Builder-Version: 1`（告诉 CLI 用经典 `POST /build`） |
| GET | `/version` | `version()`（376-394） |
| POST | `/auth` | `auth()`（723-733） |
| POST | `/kiri/wake` `/kiri/stop` `/kiri/sleep` | 窗口的 Start/Stop |
| GET/POST | `/kiri/proxy`、PUT `/kiri/proxy/system` | 代理设置 |
| GET/POST | `/kiri/resources` | VM CPU/内存 |
| GET/POST | `/kiri/domains` | 域名路由端口 |
| POST/GET/DELETE | `/kiri/usb*` | USB 直通 |
| POST | `/kiri/containers/{id}/ports` | 补发布端口 |
| GET | `/kiri/containers/{id}/listening` | 容器监听的 TCP 端口 |
| POST/DELETE | `/kiri/k8s/forward` | k8s 端口转发 |
| POST | `/kiri/containers/{id}/env` | 改环境变量 |
| GET/POST | `/kiri/images/layers` `/scan` `/scans` `/sbom` | 镜像层与漏洞扫描 |
| GET | `/info` | `info()`（396-438） |
| POST | `/build` `/build/cancel` `/build/prune` | 托管 BuildKit |
| GET/POST | `/kiri/builder` | builder 设置 |
| GET | `/images/json` | `images()`（459-487） |
| POST | `/images/create` | `pull()`（777-813） |
| POST | `/images/load` | `image_load()` |
| GET | `/images/get` | `image_save()`（565-589，chunked） |
| POST | `/containers/create` | `create()`（1450-1638） |
| GET | `/containers/json` | `list()`（1872-1927） |
| GET/POST | `/networks` `/networks/create` `/networks/prune` | |
| GET/POST | `/volumes` `/volumes/create` `/volumes/prune` | |
| GET | `/images/{name}/json` `/get` | inspect / save |
| POST | `/images/{name}/push` `/tag` | |
| DELETE | `/images/{name}` | |
| GET | `/containers/{id}/json` | `container_inspect()`（1839-1870） |
| POST | `/containers/{id}/start` `stop` `restart` `kill` `wait` `attach` `resize` `exec` | |
| GET | `/containers/{id}/logs` | `logs()`（2353-2437） |
| HEAD/GET/PUT | `/containers/{id}/archive` | `archive_get`/`archive_put`（`docker cp`） |
| DELETE | `/containers/{id}` | |
| GET/DELETE | `/volumes/{name}` | |
| GET/DELETE | `/networks/{id}` | |
| POST | `/exec/{id}/start` `resize`；GET `/exec/{id}/json` | |

`create()`（1450-1638）是**最长最复杂的函数**，逐段：解析 Image → NetworkMode 归一（`default`→`bridge`）→ `container:` 模式校验 → `EndpointsConfig` 多网络 + 别名 + 静态 IP/MAC 校验 → `Links` → `ExtraHosts`（`host-gateway` → `HOST_ADDR`）→ `hostopts::from_host` + `guest_lines` + `check_device_requests` → `StopSignal` 校验 → 镜像存在性 + 平台匹配（Intel Mac 拒绝无 amd64 的镜像）→ 读镜像 config → `healthcheck_from` → `mounts()`（1065-1226，最长）→ `port_bindings()`（909-954）→ `command()`（entrypoint/cmd 合并规则）→ env 合并（镜像 env + 代理 + 用户 env 覆盖）→ 组装 `Spec` → restart policy 校验 → `engine.create()`。

其他关键函数：`caps_from`（1400-1423，默认 cap 集 + add/drop）、`limits_from`（1425-1448）、`mounts`（1065-1226，bind/volume/tmpfs/匿名卷 + Mac 路径映射 + 强制创建卷）、`port_bindings`（909-954，`-p`/`-P` 解析 + 冲突检测）、`parse_signal`（2678-2708，信号名/数字）、`go_file_mode`（2484-2505，tar 头 mode 转换）、`upload_compression`（2567-2575，gzip 嗅探）。

#### `engine.rs`（4435 行，状态机）

`Engine`（520-591）字段即全部状态：

```
volumes / link(Option<Arc<VmLink>>) / waking / last_activity / sleeping / stopped
store / networks / state(Mutex<State>) / changed(Condvar) / layers / records / saving
held_restarts / power / parking / ceiling / cpus / memory_target / memory_used
memory_stall / oom_kills / cpu_permyriad / events / k8s_lb / k8s_routes / k8s_fwd
```

`State`（492-507）：`containers: HashMap<String, Container>`、`health`、`vm_recovery_at`。

`Container`（355-408）：`id/name/layers/created/status/started/finished/exit_code/oom_killed/vm_stopped/stopped_by_user/restart_count/restart_pending/exits/size/ip/pid/forwards/live/early_input/early_close/paused/extra_ips/extra_ifs/ip6/extra_ip6s/log/attached/spec`。

`Spec`（152-270）：镜像、entrypoint/cmd、env、cwd、user、auto_remove、privileged、limits、caps、cap_add/drop、security_opt、readonly_rootfs、hostname、stop_signal/timeout、healthcheck、shell、init、extra_networks、log_config、network_mode、ipv4/ipv6/mac、dns/dns_search/dns_options、domainname、links、labels、tty、open_stdin、console_size、aliases、extra_hosts、ports、exposed、host_opts、mounts、mount_info、volumes、anon_volumes、restart。

关键方法（按调用链）：

| 方法 | 行 | 职责 |
|---|---|---|
| `boot` | 654-699 | 读 `containers.json`、`Networks::load`、`Volumes::load`，起 balloon/cpu_meter/sleeper/clock/k8s_lb 五个后台线程 |
| `vm` / `vm_inner` | 702-797 | 拿 VM；睡着则唤醒（~0.1s）；`stopped` 或 `SHUTTING_DOWN` 则报错；recovery 模式有退避 |
| `recover_vm` / `run_vm_recovery` | 811-895 | VM 崩溃后重建 + 按 restart policy 拉起容器，退避 100ms→1min |
| `start_sleeper` / `sleep_if_idle` / `sleep_now` | 1043-1250 | 60s 空闲休眠；`sleep_now` 拿 `power` 锁停全部容器 |
| `stop_all` / `unstop` / `start_held` / `retry_held` | 1124-1203 | 窗口 Stop/Start 与 held 重启队列 |
| `load` | 1272-1369 | 反序列化 `Saved`；`was_running` 的容器标成 `Exited` + `exit_code=255` + `vm_stopped`；清理孤儿容器目录；按 `joins_others` 排序重启 |
| `save` | 1372-1404 | 写 `containers.json`（tmp + rename），跳过 `--rm` |
| `on_event` | 1406-1628 | 处理 guest 事件：`Output`（写 logfile + 广播给 attach）、`Exited`（记状态、restart policy、`--rm` 删除、health 停止） |
| `start_balloon` / `grow_memory` | 1635-1690 | 内存气球：按 `memory_used` 与 `ceiling` 调 `set_memory_target` |
| `start_clock` / `start_cpu_meter` | 1728-1771 | 每秒 `SetTime`；CPU 占用采样 |
| `create` | 1820-1890 | 生成 id、校验名字唯一、写 `Spec`、`save` |
| `start` / `start_inner` / `start_on_vm` | 1905-2233 | **核心**：见下 |
| `emit` / `subscribe` | 2236-2267 | `docker events`（保留 512 条，队列 256） |
| `link_hosts` | 2347-2381 | `--link` 生成 /etc/hosts 行 |
| `fail_start` | 2386-2443 | 启动失败：释放 IP、记 `Exited`、发事件 |
| `attach` / `logs` / `input` / `close_stdin` / `resize` | 2607-2694 | 流式 I/O |
| `wait` | 2698-2718 | `docker wait`（条件变量） |
| `connect` / `disconnect` | 2775-2852 | 网络接入/断开 |
| `pause` / `update` / `stats` / `top` / `listening` | 2855-2938 | |
| `set_ports` / `set_env` / `rename` | 2944-3022 | |
| `stop` / `stop_as` / `kill` / `remove` | 3043-3201 | 停止（先 SIGTERM 等 timeout 再 SIGKILL）、删除（含卷、网络、日志） |
| `volume_*` | 3203-3271 | 卷 CRUD |
| `exec_create` / `exec_start` / `exec_console` | 3274-3387 | exec 生命周期 |
| `shutdown` | 3453-3473 | 关 VM、落盘 |

`start_on_vm`（1955-2233）逐段：
1. 锁内解析 id、检查 VM 未断、检查 `stopped`/`parking`、检查 restart 语义、置 `Running`
2. `fresh_bind_sources`（bind 源是符号链接则每次启动重新解析）
3. `--network container:<id>` → 复用目标容器的 hostname/domainname/ns
4. 网络：`host` → `NET_HOST`；`bridge` → 分配 IPv4（`reserve` 或 `allocate_avoiding`）+ IPv6 + `DNS_GATEWAY` + 别名；其他 → `NET_NONE`
5. flags：`REMOVE`/`TTY`/`STDIN`/`PRIVILEGED`/`INIT`/`security_flags()`
6. `/etc/hosts` 默认两条：`host.docker.internal host.kiri.internal`、`gateway.docker.internal`
7. `net_opts` + `host_opts`（`guest_opts` 校验 sysctl 与 netns 冲突）
8. `restore_layers`（确保层已解包）
9. `vm.run(...)` → 拿 pid
10. TTY 则 `vm.resize`
11. `attach_extra`（额外网络）
12. `ports::start`（发布端口；冲突则 SIGKILL 并报 Conflict）
13. 投递早到的 stdin
14. `health_start`、`save`、`emit_container("start")`

#### `vmlink.rs`（674 行，宿主↔guest 连接）

`VmLink`（71-83）：一条 vsock 连接，被所有 daemon 线程共享。

- `Writer`（91-127）：`write` / `write_request`，**单写者**保证帧不交错
- `Windows` / `Credits`（133-200）：**信用窗口**——每个进程的 stdin 有 `INPUT_WINDOW` 额度，guest 回 `InputConsumed` 才补充；`lose`/`finish_loss` 处理断连
- `call`（311-332）：发请求 + 等回复（按 kind 匹配）
- 其余方法一一对应 `Msg` 变体：`usage`、`has_layer`、`send_layer`、`run`、`ack`、`update`、`freeze`、`container_stats`、`top`、`listening`、`usb_nodes`、`route`、`net_attach`/`net_detach`、`exec`、`input`、`close_stdin`、`resize`、`uplink`、`publish`/`unpublish`、`remove_volume`、`remove_bridge`、`set_time`、`retain`/`retain_layers`、`kill`、`remove`、`set_memory_target`、`listen`/`connect`、`fs_stat`/`fs_list`/`fs_read`、`stats`、`shutdown`、`retire`/`hold_retirement`、`wait_stopped`
- `Reply`（23-40）/`Event`（61-69）：回复与异步事件的区分

#### `network.rs`（1011 行，网络与 IPAM）

`Network`（22-66）：`name/driver/bridge/subnet/prefix/gateway/ip_range/aux_addresses/internal/no_ipv4/v6/labels/created`。
`V6`（70-85）：IPv6 子网、网关、地址池。
`Networks`（274-281）：`HashMap<String, Network>` + 持久化。

关键函数：`builtin()`（316-339，`bridge`=`kiri0` 172.17.0.0/16、`host`、`none`）、`parse_ipam_both`（477-539，`--subnet`/`--gateway`/`--ip-range`/`--aux-address` 校验）、`Networks::create`（598-683，分配不冲突的 /16 或 /64）、`allocate_avoiding`（191-204）、`check_static`（209-225，Docker 的静态地址规则：不能是网络/广播/网关）、`free_subnet`（746-756）、`host_routes`（787-802，宿主侧路由）。

#### `ports.rs`（1253 行，端口发布）

`Binding`（181-189）：`host_ip/host_port/container_port/proto`。
`Forwards`（192-203）：`wake`（pipe 唤醒）、`threads`、`kernel`（nftables DNAT 句柄）、`nat`（VM 内 NAT 端口）、`bound`。

`start`（383-498）流程：
1. 对每个 binding 的每个监听地址（`listen_addrs`，345-350）`bind_socket`
2. 端口被占 → `holder()`（358-367，`lsof` 找占用者）拼 Docker 原文错误
3. 端口被 Kiri 自己的域名路由占用 → `domains::step_aside` 让出后重绑
4. Linux 宿主：`vm.publish()` 走内核 DNAT（`kernel_forwards`），只有 loopback 目标才保留用户态 relay
5. macOS：`publish_nat`（502-520）让 guest 内核从 NAT 网络自己的端口转发
6. 其余：每 binding 一个线程 `accept_loop`（636-672）→ `relay`（705-758）→ `splice_all`（814-849，零拷贝）

其他：`tcp_permit`（156-178，连接数配额）、`nat_budget`（122-127，DNAT 端口预算）、`udp_loop`（541-604）、`Deframer`（868-899，帧化 relay）、`serve_ssh_agent`（990-1012）、`serve_engine_api`（1020-1039）、`listening`（1046-1076）。

#### `logfile.rs`（797 行，日志落盘）

记录格式：`[len u32][stream u8][time ns u64][line][len u32]`（大端，`len = 9 + 行长`）——Docker `local` 驱动的形状，尾部长度让 `--tail` 能从文件末尾倒走。

`DEFAULT_MAX_SIZE = 10 MiB`、`DEFAULT_MAX_FILE = 3`、`MAX_LINE = 16 KiB`（超长切分）、`FLUSH_BYTES = 32 KiB`、`FLUSH_AFTER = 1s`（不 fsync）。

`--rm` + 有客户端 attach 的容器：最新 1 MiB 留在内存（`keep_in_memory`），因为文件会随容器删除。

#### 其余 daemon 模块

| 文件 | 关键函数 | 职责 |
|---|---|---|
| `ops.rs` | `route`（23-65）、`stats_json`、`stats`、`top`、`update`、`changes`、`export`、`commit`（265-342）、`container_prune`、`image_prune`、`system_df`（437-514）、`history`、`search`、`connect`/`disconnect`、`events`（622-665） | create/start/stop 之外的 Engine API |
| `volumes.rs` | `Volumes::load/save/ensure/remove`、`local_options`（111-140，`type=nfs/tmpfs/none/cifs`）、`mount_options`、`cifs_device` | 命名卷 |
| `builder.rs`（52KB） | `ensure_image`、`create`、`ensure_running`、`watch`（326-363，空闲暂停）、`build`（718-770）、`run`、`send_context`、`buildctl_run`（853-918）、`prune`、`cache` | 托管 BuildKit 容器 |
| `mirror.rs` | `start`、`serve`、`answer`、`manifest`、`lookup` | 本地镜像暴露成只读 registry |
| `dns.rs` | `start`、`answer`（44-82）、`serve_relay`、`relay_query`、`mac_resolvers` | `*.kiri.local` → 127.0.0.1 |
| `domains.rs` | `Router`、`start`、`start_tls`、`serve`、`route`（350-391）、`candidates`、`choose`、`web_port`、`serve_tls`（478-561） | `<name>.kiri.local` 反代 |
| `tls.rs` | `authority`、`Certs`、`server_config`、`trust`/`untrust`、`trusted` | 自建 CA（name constraints） |
| `nfs.rs`（34KB） | `start`、`mount`、`unmount`、`Handles`、`Server`、`Attr` | NFSv3 over unix socket |
| `health.rs` | `Healthcheck`、`merge`、`Health`、`go_duration`、`cap_output` | healthcheck 规则 |
| `hostopts.rs` | `from_host`、`guest_lines`（76-137）、`check_device_requests`、`ipv6_disabled`、`sysctl` | HostConfig 校验与下发 |
| `binds.rs` | `detect`、`split`、`source`（132-167）、`engine_socket` | Windows 路径 / docker.sock 归一 |
| `access.rs`（28KB） | `Gate`、`Seat`、`grant`、`admit`、`set`（ACL）、`member_of` | Linux 宿主 socket 鉴权 |
| `usb.rs` | `attach`、`detach`、`list`、`watch`（389-440，设备掉线重连） | USB 直通宿主侧 |
| `proxy.rs` | `effective`、`system`、`parse_scutil`、`for_registry`、`container_env` | 代理 |
| `pulls.rs` | `Job`、`Follower`、`join` | 并发 pull 合并 |
| `filters.rs` | `Filters`、`name_matches` | Docker filters |
| `names.rs` | `random` | `adjective_scientist` |
| `time.rs` | `rfc3339`、`unix`、`parse_unix`、`civil_from_days` | 时间格式 |
| `vmcpu.rs` | — | VM CPU 采样 |
| `hostetc.rs` | `all`、`file`、`contents` | Mac 上给容器 bind 的 /etc 文件 |

### 2.6 `crates/kiri/src`（CLI 与 UI）

| 文件 | 职责 |
|---|---|
| `main.rs` | 子命令分发：`daemon`/`bridge`/`hostrelay`/`host-access`/`raw`/`bench`/`boot`/`setup`/`share`/`build-with`/`k8s`/`machine`/`usb`/`import`/`update`/`diag`/`ui`，其余交给 `docker::run` |
| `docker.rs` | `kiri run` = `docker --context kiri run` |
| `client.rs` | 窗口与 CLI 走 unix socket 的 HTTP 客户端 |
| `guest.rs` | 一个运行中的 guest（VM + agent 连接 + 启动计时） |
| `run.rs` | `kiri raw run`：无 daemon，自己起 guest 跑一个容器 |
| `images.rs` | `kiri raw pull/images/rmi` |
| `setup.rs`（42KB） | docker context、LaunchAgent、`/var/run/docker.sock` 链接、`/etc/resolver` |
| `update.rs` | 自更新（SHA-256 + 签名） |
| `k8s.rs` / `kubeconfig.rs` | k3s 单节点集群 + kubectl context |
| `machine.rs` | 长驻 Linux 机器（容器形态） |
| `share.rs`（93KB） | 公网 HTTPS 隧道 |
| `scan.rs` | Grype 漏洞扫描 + CycloneDX SBOM |
| `builds.rs` / `buildwith.rs` | BuildKit 历史 / `docker build` 用哪个 builder |
| `import.rs` | 从 OrbStack / Docker Desktop 迁移 |
| `usb/*` | USB 直通（mac/helper/usbip/fake/sysfs） |
| `diag/*` | 日志、崩溃报告、脱敏 |
| `ui/*` | 原生窗口（AppKit / GTK + WebKit），页面是 `index.html`（793KB） |
| `bridge.rs` / `hostrelay.rs` / `wsl.rs` | WSL2 桥接 |
| `flavor.rs` / `stale.rs` / `cli_pins.rs` / `tools.rs` / `appwatch.rs` | 开发版隔离 / 旧版本检测 / CLI 版本钉 / Kiri Lite 下载 / app 被删检测 |

### 2.7 `crates/kiri-win`（Windows 侧）

`main.rs`（npipe 服务）→ `serve.rs`（accept）→ `pipe.rs`（消息模式命名管道，go-winio 兼容）→ `bridge`（`wsl.exe -u root -- kiri bridge`，多路复用）。`engine.rs`（61KB）是窗口的引擎客户端；`setup.rs`（37KB）首次运行向导；`term.rs`（ConPTY）；`dns.rs`（NRPT 规则）；`usb.rs`（usbipd-win）；`wslconfig.rs`（`.wslconfig` 的 CPU/内存）。

---

## 3. 关键设计决策（gobox 取舍见 §4.2）

| 决策 | 原因 | Go 侧是否照搬 |
|---|---|---|
| **一个 VM 跑所有容器** | 隔离边界在 VM，容器间共享 guest 内核 | 暂不：共享宿主内核；VM 作为 M5 可选后端 |
| **guest PID 1 是 agent 本身** | 省掉 init + 省掉 runc/shim/dockerd，启动 2.5ms | **照搬**：`gosandbox-inner --agent`（§6.3） |
| **单线程 poll 循环** | `clone3` 不带 `CLONE_VM` 要求单线程 | **不照搬**，Go 用 goroutine + 单写者 channel |
| **手写 netlink / nf_tables** | 零依赖静态二进制 | **不照搬**：默认无网，出网交给 pasta（§6.5） |
| **手写 HTTP/1.1** | 只要 Docker 客户端那点特性 | **不照搬**，用 `net/http` + `http.Hijacker` |
| **帧 + 裸 blob 分离** | 大层不复制进帧 | **照搬** |
| **stdin 信用窗口** | 防止 guest 为不读的进程无限缓冲 | **照搬** |
| **层解包用 `<hex>.done` 标记** | 目录出现即完整，崩溃可恢复 | **照搬** |
| **overlayfs + pivot_root** | 分层 + 不可逃逸 | **照搬语义**，由 bwrap `--overlay` 完成（§6.2） |
| **`openat2(RESOLVE_IN_ROOT)`** | 符号链接不能逃出容器根 | **照搬**：解包与 agent 文件服务用 `os.Root` |
| **vsock 永久拒绝** | vsock 不做命名空间隔离，是通往宿主的后门 | **照搬**（现有 seccomp 已拒绝） |
| **cgroup-v2 BPF 设备白名单** | 否则容器可 `mknod` 读宿主磁盘 | 不需要：rootless userns 内建不了真设备节点 |
| **`containers.json` 全量重写** | 简单、原子（tmp+rename） | **照搬**：`boxes/<name>/state.json`；元数据仍在 systemdb |
| **日志用 Docker `local` 格式** | `--tail` 可从尾部倒走 | **照搬** |
| **端口发布优先内核 DNAT** | 用户态 relay 慢数倍 | 不需要：入站只经平台代理 + agent relay |

---

# 第二部分：gobox —— 基于 gosandbox 的轻量容器管理平台方案

> 本部分取代旧版「Go 版 kiri 移植路线」。结论：**不移植 kiri，借它的运行时模型**，把 gosandbox 从「单命令沙盒」升级为「每沙盒一个 rootless 容器」，服务 E2B 兼容 API 与 Agent 工具调用。

## 4. 容器专家视角：定位与取舍

### 4.1 我们要的是什么

| 维度 | Docker Desktop / kiri | 本平台（下称 gobox） |
|---|---|---|
| 用户 | 开发者跑服务 | Agent / E2B SDK 跑不可信代码 |
| 接口 | Docker Engine API | 现有 E2B 控制面 + envd 数据面（`internal/api/router.go:mountE2BRoutes`） |
| 生命周期 | 服务常驻、restart policy | 会话式：创建 → 交互 → 暂停 → 恢复 → 过期回收 |
| 镜像 | 任意镜像 + build + push | **模板镜像**：拉取 / 导入 + 从沙盒快照生成 |
| 网络 | 完整 bridge / DNS / 端口 | 默认无网；可选「受控出网」；入站只走平台代理 |
| 特权 | daemon 常驻（kiri 在 Linux 上是 root） | **全程 rootless**，服务进程不需要 root |
| 规模 | 单机数十容器 | 单机数百空闲沙盒（密度优先） |

### 4.2 从 kiri 借什么、不借什么

| kiri 设计 | 借鉴 | 理由 |
|---|---|---|
| guest 里的 agent 当 PID 1，宿主只发帧 | **借** | 让沙盒有「常驻进程表」：后台进程跨 API 调用存活、exec 进同一命名空间、文件操作在沙盒视角执行。也是以后上 VM 的唯一迁移点 |
| `kiri-proto` 帧格式 + 裸 blob + stdin 信用窗口 | **借** | 大文件不进帧、慢读进程不撑爆内存 |
| 层解包的逃逸防护、膨胀上限、`.done` 标记 | **借** | 镜像是外部输入，安全要求与 kiri 相同 |
| seccomp 默认表 + vsock / io_uring 永久拒绝 | **借** | kiri 的 `denied()` 直接对齐 Docker 默认 profile |
| `CLONE_INTO_CGROUP` 原子入组 | **借** | Go 的 `SysProcAttr.UseCgroupFD` 就是它，修掉现有「先启动再写 cgroup.procs」的竞态 |
| logfile 的 `local` 记录格式 | **借** | `--tail` 可从尾部倒走 |
| 空闲休眠（sleeper） | **借，改成 freeze** | 先 `cgroup.freeze`（内存留着，恢复毫秒级），久了再 stop |
| 自己 clone3 + 挂载编排 | **不借** | bwrap 已经做对了，且 0.10+ 支持 overlay；自己写是最大的安全风险源 |
| 手写 netlink / nf_tables / HTTP / DNS | **不借** | 默认无网，出网交给 pasta |
| Docker Engine API、compose、build、push | **不借** | 用户不是开发者 |
| bridge / veth / IPAM / DNAT | **不借** | 沙盒之间不需要互通；入站用 agent relay（见 §6.5） |
| NFS / domains / TLS CA / USB / k8s / machine / share / UI | **不借** | 与沙盒无关 |
| 一个 VM 装所有容器 | **暂不借** | 共享内核是已接受的风险；VM 作为 M5 的可选后端，复用同一 agent |

### 4.3 设计原则

1. **rootless 优先**：只依赖 user namespace；任何需要 root 的能力（多段 uid 映射、磁盘配额）都是可选增强，探测不到就在 `Capabilities` 里标明。
2. **fail-closed 不变**：沿用 `ErrIsolationUnavailable` 语义，镜像模式建立不了就拒绝，不退回 host 模式。
3. **一个沙盒 = 一个 bwrap 进程树 + 一个 agent**：没有 shim、没有常驻 daemon；服务进程就是 supervisor。
4. **镜像只读共享，可写层私有**：lower 按镜像共享，upper + workspace 每沙盒一份。
5. **纵深防御**：userns → mount/pid/ipc/uts/net ns → Landlock → seccomp → caps → no_new_privs → cgroup，任何一层缺失都写进 `Warnings`。
6. **与现有代码兼容**：`vm.Driver` 接口不破坏；新能力用可选接口（与现有 `vm.Interactive` 同一模式）；`vm/local` 保留为 `image=host` 后端。

---

## 5. 架构

### 5.1 分层

```
┌──────────────────────────────────────────────────────────────────────┐
│ internal/api      E2B 控制面 / envd 数据面（现有）                     │
│                   + 解锁：pause(真冻结) snapshots fork logs metrics     │
│                     network WatchDir 端口访问                          │
├──────────────────────────────────────────────────────────────────────┤
│ internal/sandbox  Manager（现有）：元数据、锁、TTL、reaper、procTable   │
│                   + 镜像/模板管理、空闲冻结、快照元数据                  │
├──────────────────────────────── vm.Driver ───────────────────────────┤
│ gosandbox/vm/local   现有：image=host，宿主根只读 + 工作区             │
│ gosandbox/vm/box     新增：镜像根，每沙盒常驻 agent                    │
├──────────────────────────────────────────────────────────────────────┤
│ gosandbox/image      新增：引用解析、拉取、导入、blob store、解包、GC    │
│ gosandbox/rootfs     新增：快照器（bwrap overlay / 拷贝回退）           │
│ gosandbox/proto      新增：宿主↔agent 帧协议（kiri-proto 子集）         │
│ gosandbox/box        新增：Supervisor（拉起 bwrap、持有控制连接、cgroup）│
│ gosandbox/netx       新增：出网（pasta）+ 出网白名单                    │
│ gosandbox/host       现有：Policy / bwrap 参数 / Landlock / seccomp     │
│                      扩展：RootFS、Mounts、Caps、Rlimits、SeccompProfile│
├──────────────────────────────────────────────────────────────────────┤
│ cmd (gosandbox-inner) 现有 helper → 增加 --agent 模式（沙盒 PID 1）    │
└──────────────────────────────────────────────────────────────────────┘
依赖方向：vm/box → box → {host, rootfs, image, proto, netx}；host 不 import 任何新包。
```

### 5.2 一个沙盒的运行形态

```
simplebase 进程（非 root）
 └─ box.Supervisor(sb-xxx)                       Go 对象，不是进程
     ├─ cgroup: <delegated>/gobox/sb-xxx          UseCgroupFD 原子入组
     ├─ ctrl:   socketpair 父端                    帧协议
     └─ bwrap  --unshare-all --die-with-parent
               --overlay-src <image lower> --overlay <upper> <work> /
               --bind <ws> /workspace  --proc /proc --dev /dev
               --file 3 /run/gosandbox/policy.json --fd ctrl=4
               -- gosandbox-inner --agent --ctrl 4
          └─ gosandbox-inner（PID 1，Landlock+seccomp 已加载）
               ├─ 进程表：exec / start / pty / 后台进程
               ├─ 文件服务：os.Root("/")、inotify
               ├─ 统计：/sys/fs/cgroup（只读视图）、/proc/net/tcp
               └─ relay：入站连接 → 127.0.0.1:<port>
```

与现有 `vm/local` 的根本区别：现在每次 `Exec` 都新起一个 bwrap，命令结束沙盒就「不存在」；gobox 里 bwrap 常驻，`Exec` 只是给 agent 发一帧（**热路径从 ~3ms 降到 ~0.3ms**，且后台进程可以存活）。

### 5.3 数据布局

```
<Root>/                                    sandbox.root（现有配置项）
  images/
    blobs/sha256/<hex>                     压缩层、config、manifest（原样）
    lower/<chainID>/                       按镜像链解包、已应用 whiteout 的只读根
    lower/<chainID>.done                   完整性标记（kiri layers.rs 语义）
    index.json                             引用 → manifest digest → chainID
    tmp/                                   下载中，校验后 rename
  snapshots/<snapID>/
    upper.tar.zst + meta.json              文件系统快照（可当作 lower 再解包）
  boxes/<name>/
    upper/ work/                           overlay 可写层（rootfs 改动）
    ws/                                    /workspace（与现有 vm/local 的 ws 同义）
    run/                                   policy.json、ctrl 信息、netns 句柄
    logs/<proc>.log                        进程输出（kiri logfile 格式）
    state.json                             running / paused / stopped、启动参数
  <name>/ws                                现有 vm/local 布局，保持不动
```

---

## 6. 核心机制设计

### 6.1 镜像（`gosandbox/image`）

| 功能 | 设计 | kiri 参照 |
|---|---|---|
| 引用解析 | `alpine`、`ghcr.io/o/r:tag`、`@sha256:` 归一化；Docker Hub 补 `library/` | `kiri-image/reference.rs` |
| 拉取 | 用 `google/go-containerregistry/pkg/v1/remote` 取 manifest 与 blob（成熟、支持 auth/重试/多架构）；**解包自己写** | `registry.rs::pull_with` |
| 平台选择 | 只接受 `linux/<GOARCH>`；没有匹配则报错，不跨架构模拟 | `Platform::host_linux` |
| 并发合并 | 同引用同平台 `singleflight`，后来者跟随进度 | `daemon/pulls.rs` |
| 导入 | `docker save` 格式 tar 导入（离线环境、内网分发） | `archive.rs` |
| 白名单 | `sandbox.images` 从「展示用标签」升级为「允许拉取的引用前缀」；默认只允许已导入镜像 | — |
| 存储 | 内容寻址；下载写 `tmp/`，校验 sha256 后 rename | `store.rs::blob_writer` |
| GC | 引用计数（index + 运行中沙盒 + 快照），无引用的 lower 延迟删除 | `RetainLayers` |

**解包是安全关键路径**，逐条照抄 kiri `layers.rs` 的防护：

1. 所有写入走 `os.Root(lowerTmp)`（Go 1.24+，底层是 `openat2(RESOLVE_IN_ROOT)` 语义），拒绝绝对路径与 `..`，**中间段是符号链接也不能逃逸**（kiri `symlink_parent_cannot_escape_layer`）。
2. 膨胀上限：解压字节数 ≤ 压缩大小 × 32 且 ≤ 配置上限（kiri `Capped`）。
3. 条目数上限、单文件大小上限、路径长度上限。
4. 设备节点、FIFO 跳过（rootless 本来也建不了）；setuid/setgid 位清掉（no_new_privs 已兜底，这是纵深）。
5. 硬链接目标必须已在同一 lower 内。
6. 完成后写 `<chainID>.done`，**没有标记的目录视为半成品，启动时删除**。

**rootless 下的 whiteout**：非 root 建不了 0:0 字符设备，所以不按层存 lower，而是**按镜像链（chainID）把所有层压平成一个 lower**，解包时直接执行 `.wh.<name>` 删除与 `.wh..wh..opq` 清空。代价是不同镜像不共享层（对「少量模板镜像」的场景可接受）；收益是 overlay 只有一层 lower、无需特权。

**属主**：bwrap 只能做单段映射（外部服务 uid ↔ 沙盒内 0），所以镜像里所有文件在沙盒内都属于 root。需要多用户的镜像（postgres 的 999 等）列为已知限制，M5 再评估 `newuidmap` + `/etc/subuid`。

### 6.2 根文件系统（`gosandbox/rootfs`）

按探测结果选择快照器，结果写进 `Caps.Snapshotter`：

| 快照器 | 条件 | 创建耗时 | 说明 |
|---|---|---|---|
| `overlay` | bwrap ≥ 0.10（`--overlay-src` / `--overlay`）且内核 ≥ 5.11（userns 内 overlayfs） | ~ms | 首选。upper/work 在 `boxes/<name>/` |
| `copy` | 任意 | 与镜像大小成正比 | 回退：把 lower 拷贝（同文件系统优先 reflink）到 `boxes/<name>/rootfs`，以 `--bind` 作根 |
| `host` | — | 0 | 现有 `vm/local` 行为，`image=host` |

探测方式与现有 `probeLinux` 一致：真实跑一次 `bwrap --overlay-src ... -- /bin/true`，失败就降级，降级写 `Warnings`。

挂载顺序（扩展 `BwrapArgs`，固定顺序便于测试）：

```
--overlay-src <lower> --overlay <upper> <work> /     根
--bind <ws> /workspace                               工作区
--tmpfs /tmp  --size <shm> --tmpfs /dev/shm          临时目录（--size 需 bwrap ≥ 0.9）
--proc /proc  --dev /dev                             新 procfs / 最小 dev
--ro-bind /dev/null <masked...>                      Docker masked paths
--remount-ro /proc/sys 等                            Docker readonly paths
--file 3 /run/gosandbox/policy.json
```

Docker 默认 masked / readonly 路径一并照抄：`/proc/kcore /proc/keys /proc/timer_list /proc/sched_debug /proc/acpi /proc/scsi /sys/firmware`（masked），`/proc/bus /proc/fs /proc/irq /proc/sys /proc/sysrq-trigger`（readonly）。

### 6.3 Agent：沙盒 PID 1（`cmd` 的 `--agent` 模式）

**为什么必须有 agent**：bwrap 进程树一旦建立，宿主无法再「把新进程放进同一组命名空间」（rootless 下 `setns` 进别人的 userns 需要该 userns 的权限，等同于再造一套 nsenter）。kiri 的答案是让 PID 1 自己 fork——这里照做。

| 职责 | 实现 | kiri 参照 |
|---|---|---|
| 收尸 | PID 1 必须 `wait4(-1)`，否则僵尸堆积 | `agent.rs::reap` |
| 进程表 | `id → *exec.Cmd + 输出泵 + stdin 窗口`；exec 默认独立会话 / 进程组 | `Proc` |
| PTY | 在沙盒自己的 `/dev/pts` 里开（bwrap `--dev` 带 devpts），`setsid` + `TIOCSCTTY` | `finish_child` 的 TTY 分支 |
| 信号 | 任意信号发给进程组（现有 `host.Proc` 只允许 TERM/KILL，是因为宿主侧信号可能误伤；agent 在沙盒内无此顾虑） | `Kill` |
| 用户 | `uid/gid` 只能是映射内的值（单段映射下只有 0）；保留字段以便 M5 | `resolve_user` |
| rlimits | fork 后 `prlimit`（nofile、nproc、core=0、fsize） | `hostopts::set_rlimits` |
| 文件 | `os.Root("/")`：读写、列目录、stat、删除、mkdir、rename；**在沙盒视角执行**，能访问整个 rootfs 而不只是 workspace | `fsview.rs` + `copy.rs` |
| 监听 | inotify → `WatchDir` 事件流（解锁 envd 的 `FsUnimplemented`） | — |
| 统计 | 读 `/sys/fs/cgroup/{memory.current,memory.stat,cpu.stat,pids.current,io.stat,memory.events}` | `cgroup.rs::stats` |
| 端口 | 解析 `/proc/net/tcp{,6}` 里 LISTEN 的端口 | `ListenQuery` |
| relay | 宿主为每条入站连接开一条流，agent 连 `127.0.0.1:<port>` 双向拷贝 | `relay.rs` |
| 时钟 | 不需要（共享宿主内核时钟） | `SetTime` |

**seccomp 在镜像模式下要放宽 `AF_UNIX`**。现有 `SeccompFilter` 拒绝 `socket(AF_UNIX)` 与 `connect/bind/listen/accept`，原因是 host 模式下宿主根可见，命令能连 `docker.sock`、ssh-agent。镜像模式下宿主文件系统不可见、网络在独立 netns，这条理由不成立，反而会让 postgres、redis、python `multiprocessing.Manager`、任何本地 IPC 全部失败。镜像模式改用 kiri 的 `denied()` 表（对齐 Docker 默认 profile）+ `AF_VSOCK` / io_uring 永久拒绝；host 模式维持现状。

**控制通道**：supervisor 建 `socketpair`，子端经 bwrap `ExtraFiles` 传入。agent 只在已有 fd 上 `read/write`，不需要 `socket/connect`，所以 seccomp 加载后控制通道依然可用。

### 6.4 宿主↔agent 协议（`gosandbox/proto`）

帧：`len u32 LE | kind u16 LE | payload`，`MAX_FRAME = 1 MiB`，`MAX_OUTPUT_CHUNK = 64 KiB`，`INPUT_WINDOW = 4 MiB`，首帧 `Hello{protocol, agent_version, boot_ns}`，版本不符直接拒绝（kiri `PROTOCOL` 的做法）。

只取 kiri `Msg` 的子集：

| 组 | 消息 |
|---|---|
| 会话 | `Hello` `Ping/Pong` `Shutdown` `Error` `Ack` |
| 进程 | `Exec{id, argv, env, cwd, user, tty, stdin, rows, cols, rlimits}` → `Started{id,pid}` → `Output{id,stream,data}*` → `Exited{id,code,signal,oom}`；`Input` `CloseStdin` `Resize` `Signal` `InputConsumed` |
| 文件 | `FsStat` `FsList` `FsRead{offset,len}` `FsWrite`（后接裸字节）`FsRemove` `FsMkdir` `FsRename` `FsWatch/FsUnwatch` → `FsEvent` |
| 观测 | `StatsQuery` → `Stats`；`TopQuery` → `Top`；`ListenQuery` → `Listening` |
| 中继 | `Relay{conn_id, port}` → 之后该 conn_id 的 `Data` / `Close` 帧 |

Go 实现要点：**单写者**（所有帧经一个 `chan` 进同一个 writer goroutine，否则帧交错），读侧一个 goroutine 按 `id` 分发。不追求零分配，用 `sync.Pool` 复用 64KiB 缓冲即可。

### 6.5 网络（`gosandbox/netx`）

三档，默认 `none`：

| 模式 | 出站 | 入站 | 依赖 |
|---|---|---|---|
| `none` | 无（netns 只有 `lo`） | 经 agent relay（见下） | 无 |
| `egress` | pasta（passt）提供 rootless 用户态出网 | 同上 | `pasta` 二进制（podman 5 默认网络栈） |
| `egress-allowlist` | 同 egress，netns 内 nftables 默认拒绝，只放行白名单域名解析出的地址 | 同上 | 同上 |

**入站不需要任何网络栈**：E2B 的 `getHost(port)` 由 SimpleBase 反向代理实现，代理收到请求后经控制通道发 `Relay{port}`，agent 在沙盒 netns 内连 `127.0.0.1:port`。这是 kiri vsock relay 的同构做法，`none` 模式也能暴露端口，且不会在宿主上开任何监听。

**出网必须挡的地址**（云上 SSRF 最常见的落点）：`169.254.0.0/16`（含 `169.254.169.254` 元数据）、`100.64.0.0/10`、`10/8`、`172.16/12`、`192.168/16`、`fc00::/7`、`fe80::/10`、宿主 loopback（pasta 加 `--no-map-gw`）。白名单模式下 agent 内跑一个最小 DNS 代理：只解析白名单域名，解析结果写进 nft set（带 TTL），这与 Codex / E2B 的网络规则同义。

### 6.6 资源与 cgroup

现状问题与改进：

| 现状（`host/cgroup_linux.go`） | 问题 | 改进 |
|---|---|---|
| `c.Start()` 之后再写 `cgroup.procs` | 窗口内 fork 出的进程逃出 cgroup（代码注释已承认） | `SysProcAttr{UseCgroupFD: true, CgroupFD: fd}`，内核 `CLONE_INTO_CGROUP` 原子入组 |
| 只在「当前 cgroup 已委派」时生效 | 大多数部署 `Enforced=false` | 部署文档要求 systemd unit `Delegate=yes`；启动时在委派子树下建 `gobox/` 并开启 `memory cpu pids io` |
| 只有 memory.max / cpu.max / pids.max | — | 增加 `memory.high`（软限，先回收再 OOM）、`memory.swap.max=0`、`memory.oom.group=1`（OOM 杀整组，避免半死）、`io.max`、`cpu.weight` |
| 无冻结 | E2B pause 实际是 stop（`sandbox_handler.go::Pause` 调 `svc.Stop`） | `cgroup.freeze=1` 真暂停，恢复毫秒级 |
| 无统计 | `metrics` 端点 Unsupported | 读 `memory.current/stat`、`cpu.stat`、`pids.current`、`io.stat`；`memory.events` 的 `oom_kill` 计入 `Exited.oom` |
| 磁盘无限额 | upper + ws 可以写满宿主盘 | rootless 无配额：agent 周期 `du`（kiri `usage.rs` 语义）超限 → 冻结并报 `disk_limit`；部署在 XFS 且有 root 时可选 project quota |

### 6.7 生命周期

```
          create            start                 idle > freeze_after
 (none) ────────► created ────────► running ─────────────────────────► paused(frozen)
                     ▲                │  ▲                                  │
                     │        stop    │  │ resume (thaw, ~ms)               │ idle > stop_after
                     │                ▼  │                                  ▼
                     └──────────── stopped ◄────────────────────────────────┘
                                      │ delete / TTL
                                      ▼
                                   removed
```

| E2B 操作 | 现在 | gobox |
|---|---|---|
| `pause` | `svc.Stop`（丢进程） | `freeze`，进程与内存保留 |
| `resume` / `connect` | 重新可执行（无状态） | `thaw`；若已被降级为 stopped 则冷启动，upper/ws 保留 |
| 空闲 | 无 | `freeze_after`（如 5min）→ `stop_after`（如 30min），借 kiri sleeper 思路 |
| 服务重启 | 无影响（无常驻） | `--die-with-parent` 使沙盒随之结束；启动时扫描 `state.json`，把 running/paused 记成 stopped（kiri `load()` 的 `was_running → Exited(255)` 同义），用户下次访问冷启动 |

服务重启不丢沙盒进程（detach supervisor）是可选增强，代价是要自己做孤儿回收与重连，放到 M5。

### 6.8 快照与 fork（解锁 E2B `snapshots` / `fork`）

只做**文件系统快照**，不做内存快照（CRIU 在 rootless + seccomp 下基本不可用）：

1. `freeze` → 打包 `upper/` 与 `ws/`（zstd tar）→ `thaw`，冻结窗口与 upper 大小成正比。
2. 快照 = 「父镜像 chainID + 一层 diff」，元数据进 `snapshots/<id>/meta.json`。
3. **fork** = 以快照为 lower 新建沙盒：先把父 lower 与 diff 压平成新 chainID（首次 fork 付一次成本，之后复用）。
4. **模板** = 快照 + 名字：E2B 的「模板构建」就是「在沙盒里跑安装命令再快照」，不需要 BuildKit。

### 6.9 日志与可观测

- 每个后台进程输出落 `logs/<proc>.log`，记录格式直接用 kiri logfile 的 `[len][stream][ts][data][len]`，10 MiB × 3 轮转，支持 `tail`/`since`/`follow`（解锁 E2B `logs`）。
- 现有 `internal/sandbox/output_hub.go` 继续负责实时订阅，日志文件负责回放。
- 指标：每沙盒 CPU、内存、pids、IO、磁盘、OOM 次数；平台级：沙盒数（按状态）、冷/热启动耗时分布、拉取耗时、冻结次数。

### 6.10 macOS

- 短期：macOS 只提供 `image=host`（现有 Seatbelt），`Capabilities.Images` 为空，控制台据此隐藏镜像选择。
- M5：`vm/box` 的 macOS 实现跑一个轻量 Linux VM（`Code-Hex/vz` 封装 Virtualization.framework），VM 内跑**同一个** `gosandbox-inner --agent`，传输从 socketpair 换成 vsock。这正是 kiri 的结构，协议与 agent 完全复用，这也是 §6.3 坚持做 agent 的原因。

---

## 7. 接口设计

### 7.1 `host.Policy` 扩展（向后兼容，零值即现有行为）

```go
type Policy struct {
	// ……现有字段不变……

	// RootFS 非空时以镜像根替代宿主只读根；nil 为现有 host 模式。
	RootFS *RootFS
	// Mounts 是额外挂载，按目标路径深度排序后追加（父先子后，kiri parse_mounts 的规则）。
	Mounts []Mount
	// Seccomp 选择过滤器："host"（现有清单）| "default"（Docker 默认 + vsock/io_uring 拒绝）。
	// RootFS 非空时零值取 "default"。
	Seccomp string
	// CapAdd 只允许 Docker 默认集合内的 capability；默认全部丢弃（现状）。
	CapAdd []string
	Rlimits []Rlimit
	ShmBytes int64
	// Network 取 none | egress | egress-allowlist；非 none 需要 RootFS。
	Network   string
	Allowlist []string
}

type RootFS struct {
	Lower, Upper, Work string // overlay；Upper 为空表示只读根
	Copy               string // copy 快照器的根目录，与 Lower 互斥
}

type Mount struct {
	Source, Target string
	ReadOnly       bool
	Tmpfs          bool
	SizeBytes      int64
}
```

### 7.2 `vm` 可选能力接口（与现有 `vm.Interactive` 同一模式）

```go
type Pauser interface {
	Pause(ctx context.Context, name string) error
	Resume(ctx context.Context, t Target) error
}

type Snapshotter interface {
	Snapshot(ctx context.Context, name, snapID string) (SnapshotInfo, error)
	Fork(ctx context.Context, snapID string, t Target) error
	DeleteSnapshot(ctx context.Context, snapID string) error
}

type Observer interface {
	Metrics(ctx context.Context, name string) (Metrics, error)
	Logs(ctx context.Context, name, procID string, opt LogOptions) (io.ReadCloser, error)
	Processes(ctx context.Context, name string) ([]ProcInfo, error)
}

type PortDialer interface {
	// DialPort 返回一条到沙盒内 127.0.0.1:port 的连接（经 agent relay）。
	DialPort(ctx context.Context, name string, port uint16) (net.Conn, error)
}

type Watcher interface {
	Watch(ctx context.Context, t Target, path string, recursive bool) (<-chan FileEvent, error)
}

type ImageStore interface {
	Pull(ctx context.Context, ref string, progress func(PullProgress)) (ImageInfo, error)
	Import(ctx context.Context, r io.Reader) ([]ImageInfo, error)
	List(ctx context.Context) ([]ImageInfo, error)
	Remove(ctx context.Context, ref string) error
}
```

`internal/sandbox.Capabilities` 增加 `Snapshots / Fork / Pause / Metrics / Logs / Ports / Watch / Network []string / Images []ImageInfo`，由驱动实现了哪些接口决定；`router.go` 里 `fork snapshots logs metrics network` 的 `Unsupported` 按能力逐个替换为真实 handler。

### 7.3 `Spec.Image` 语义

| 取值 | 后端 | 说明 |
|---|---|---|
| `host`（默认，兼容现状） | `vm/local` | 宿主根只读 |
| `<ref>`（如 `python:3.12-slim`） | `vm/box` | 必须已拉取 / 导入且命中 `sandbox.images` 白名单 |
| `snap:<id>` | `vm/box` | 从快照创建（fork） |

`config.SandboxConfig.Backend` 增加 `box`；`auto` = 探测 `box` 可用则用，否则 `local`。

---

## 8. 安全基线（镜像模式）

| 控制项 | 设置 | 来源 |
|---|---|---|
| 命名空间 | user / mnt / pid / ipc / uts / net / cgroup 全部新建 | bwrap `--unshare-all` |
| 根 | 镜像 overlay；宿主文件系统完全不可见 | — |
| Landlock | `/` 只读（镜像根内），`/workspace` `/tmp` upper 可写 | 现有 `LandlockRules` 改为沙盒内路径 |
| seccomp | Docker 默认 profile（kiri `denied()`），`AF_VSOCK`、io_uring 无条件拒绝 | kiri `seccomp.rs` |
| capabilities | 默认全丢；`CapAdd` 仅限 Docker 默认 14 项 | kiri `default_caps` |
| no_new_privs | 恒开 | 现有 |
| masked / readonly paths | Docker 默认清单 | §6.2 |
| /dev | bwrap 最小集（null zero full random urandom tty pts ptmx shm） | 无设备 BPF 需求：userns 内本来建不了真设备 |
| 资源 | memory.max/high/swap、pids、cpu、io、oom.group，磁盘 du 监控 | §6.6 |
| 网络 | 默认无；出网挡私网与元数据；入站只经平台代理 | §6.5 |
| 镜像 | 白名单 + 解包防护六条 | §6.1 |
| 文件工具 | 全部经 agent，在沙盒视角执行；宿主侧不再直接读写 upper | 取代 `vm/local` 的 `os.Root(ws)` |
| 环境变量 | 不继承服务进程（现状） | 现有 `baseEnv` |
| 已接受风险 | 共享内核（Dirty Pipe 类内核洞）、单段 uid 映射 | VM 后端（M5）解决 |

---

## 9. 能力补齐清单

| # | 能力 | 位置 | kiri 参照 | 解锁的 E2B / 平台能力 | 优先级 | 估时 | 状态 |
|---|---|---|---|---|---|---|---|
| 1 | `UseCgroupFD` 原子入组 + 委派子树初始化 | `host/cgroup_linux.go` | `CLONE_INTO_CGROUP` | 限额真实生效 | P0 | 3d | ✅ M0 |
| 2 | memory.high / swap / oom.group / io / freeze / stats | `host/cgroup_linux.go` | `cgroup.rs` | pause、metrics | P0 | 4d | ✅ M0 |
| 3 | seccomp `default` 档 | `host/inner` | `seccomp.rs::denied` | 镜像内本地 IPC 可用 | P0 | 3d | ✅ M0 |
| 4 | masked / readonly paths、`--size` tmpfs、shm | `host/bwrap_args.go` | `mount_container_filesystems` | 加固 | P0 | 2d | ✅ M0 |
| 5 | 帧协议 | `proto/` | `kiri-proto` | 一切 agent 能力的前提 | P0 | 1w | ✅ M1 |
| 6 | agent：收尸、进程表、PTY、信号、rlimits | `cmd` + `host/inner/agent` | `agent.rs` + `finish_child` | 后台进程常驻、热 exec | P0 | 2w | ✅ M1 |
| 7 | Supervisor + `vm/box` 骨架（先用 `host` 根跑通） | `box/` `vm/box/` | `vmlink.rs` | 与现有 `vm/local` 并行可切换 | P0 | 1w | ✅ M1 |
| 8 | 镜像拉取 / 导入 / store / 白名单 | `image/` | `kiri-image` | 自定义镜像 | P1 | 2w | ✅ M2（白名单未做） |
| 9 | 解包防护 + 压平 lower + GC | `image/unpack.go` | `layers.rs` | 同上 | P1 | 1w | ✅ M2 |
| 10 | overlay / copy 快照器 + 探测 | `rootfs/` | `mount_rootfs` | 同上 | P1 | 1w | ✅ M2 |
| 11 | agent 文件服务 + inotify | agent | `fsview.rs` `copy.rs` | envd 文件 API 全覆盖、`WatchDir` | P1 | 1w | ✅ M3 |
| 12 | 进程日志落盘与回放 | `box/logfile.go` | `logfile.rs` | E2B `logs` | P1 | 4d | ✅ M3（已接 API） |
| 13 | 空闲冻结 / 降级停止 / 重启恢复 | `internal/sandbox` | sleeper + `load()` | 密度 | P1 | 4d | ✅ M1/M3 |
| 14 | agent relay + 平台反向代理 | agent + `internal/api` | `relay.rs` `ports.rs` | `getHost(port)` | P1 | 1w | ✅ M3 |
| 15 | 文件系统快照 / fork / 模板 | `box/snapshot.go` | `commit` | E2B `snapshots` `fork`、模板 | P2 | 1.5w | ⬜ |
| 16 | 出网（pasta）+ 私网拦截 | `netx/` | — | E2B `network` | P2 | 1w | ✅ M4 |
| 17 | 出网白名单（DNS 代理 + nft set） | `netx/` + agent | `dns.rs`（思路） | 受控出网 | P2 | 1w | 🟡 代理已实现；nft set 写入未接 |
| 18 | 磁盘用量监控与限额 | `vm/box` | `usage.rs` | 防写满 | P2 | 3d | ✅ M4（宿主侧 du，不是 agent） |
| 19 | detach supervisor（服务重启不丢沙盒） | `box/` | `take_over` | 升级不中断 | P3 | 1.5w | ✅ M5 |
| 20 | 多段 uid 映射（newuidmap） | agent + `box/` | — | 多用户镜像 | P3 | 1w | ✅ M5 |
| 21 | macOS VM 后端（vz + 同一 agent） | `vm/box` darwin | `kiri-vmm/apple.rs` | macOS 镜像沙盒、VM 级隔离 | P3 | 4w | 🟡 传输层与 vsock 已就绪；VM 本体待 cgo |

合计：P0 约 5 周，P1 约 7 周，P2 约 5 周；P3 按需。

---

## 10. 里程碑

| 里程碑 | 内容（编号见 §9） | 验收 | 状态 |
|---|---|---|---|
| **M0 加固现有 host 模式** | 1 2 3(仅修清单) 4 | 「已委派」环境 `Limits.Enforced=true` 实测读回 `memory.max`；fork 炸弹被 pids 挡住；E2B `metrics` 可用 | **代码完成**（见 §10.1） |
| **M1 常驻 agent** | 5 6 7 | `image=host` 走 `vm/box`：后台进程跨请求存活；热 exec p50 < 1ms；`vm/local` 测试集全部在 `vm/box` 上通过 | **代码完成**（见 §10.2） |
| **M2 镜像沙盒** | 8 9 10 11 12 | `python:3.12-slim`、`node:22-slim` 拉取后 `pip/npm install`（需 M4 出网时）或离线导入可跑；冷启动 p50 < 100ms（overlay）；解包防护的恶意 tar 用例全部拒绝 | **代码完成**（见 §10.3） |
| **M3 E2B 能力补全** | 13 14 15 | E2B SDK 的 pause/resume、`getHost`、`logs`、snapshots、fork 端到端测试通过；`router.go` 不再有 `Unsupported` | ✅ 完成（logs / getHost / WatchDir / snapshots / fork；见 §10.4、§10.5） |
| **M4 受控网络** | 16 17 18 | 元数据地址与私网不可达；白名单外域名解析失败；磁盘超限冻结 | **代码完成**（见 §10.6；Linux 实测待补） |
| **M5 可选增强** | 19 20 21 | 按需求排期 | **多数完成**（见 §10.7；VM 本体待 cgo） |

### 10.1 M0 已交付（2026-11-17）

| 项 | 落点 | 行为 |
|---|---|---|
| cgroup 原子入组 | `host/run_linux.go:joinCgroup`、`host/start_linux.go` | `SysProcAttr.UseCgroupFD`（`CLONE_INTO_CGROUP`）替代「启动后写 cgroup.procs」；内核不支持时回退旧路径，`Limits.Enforced` 两种都如实上报 |
| 限额补全 | `host/cgroup_linux.go:apply` | 除 `memory.max`/`cpu.max`/`pids.max` 外，写 `memory.high`（硬限 90%）、`memory.swap.max=0`、`memory.oom.group=1`；文件不存在时忽略，不误报失败 |
| 固定子组 | `Policy.CgroupName`、`host/cgroup_linux.go:newNamedCgroupGroup` | 命名为 `gosandbox-<name>` 的子组在命令结束后保留（`keep` 标记），供按名字查用量与冻结；`vm/local` 在 `Exec`/`Start` 传沙盒名，`Remove` 时清理 |
| 冻结 | `cgroupGroup.freeze`、`host/metrics_linux.go:FreezeCgroup` | 读写 `cgroup.freeze` |
| 统计 | `host/metrics*.go` | 读 `memory.current/peak`、`cpu.stat`、`pids.current`、`io.stat`（按设备累加）、`memory.events.oom_kill` |
| 能力接口 | `vm/driver.go`：`Meter` / `Pauser` / `Freezer` | 与现有 `vm.Interactive` 同一模式：实现即生效，未实现则能力为假 |
| 本机后端 | `vm/local`：`Metrics` / `Pause` / `Resume` / `CanFreeze` | `CanFreeze` 探测 cgroup 是否委派并缓存，避免「声明支持却在真实环境报错」 |
| seccomp 两档 | `spec.Spec.Seccomp`、`host/inner:SeccompDefaultFilter`/`filterFor` | `host` 档保持现状；`default` 档对齐 Docker 默认 profile（允许 AF_UNIX，仍拒绝 vsock / io_uring / keyring / 模块 / 挂载 / bpf 等 40 项）。**这是镜像模式的前置条件** |
| masked / readonly | `host/bwrap_args.go:MaskedPaths`/`ReadonlyPaths`、`Policy.DisableMaskedPaths` | Docker 默认清单，路径存在才挂；排在绑定之后、命名空间之前 |
| shm / tmp 上限 | `Policy.ShmBytes`/`TmpBytes`、`Caps.BwrapSize`、`BwrapInput.SizesSupported` | 显式配置时挂带 `--size` 的 tmpfs；bwrap < 0.9 不认 `--size` 时自动退化为不限大小 |
| E2B pause | `internal/sandbox:Manager.Pause/Resume/CanPause`、handler `Pause`/`Connect` | 支持真冻结时 `pause` 只冻结不改文件；不支持则如实退回 `Stop`，`capabilities.pause=false` |
| E2B metrics | `Manager.Metrics/CanMetrics`、`GET /sandboxes/{id}/metrics` | 有 cgroup 时返回真实数值；否则 501 `unsupported_feature`（不报 0 误导调用方） |
| 空闲冻结 | `config.SandboxConfig.FreezeAfter`（默认 2 分钟）、`Manager.freezeIdle`、`systemdb.ListIdleRunningSandboxes` | reaper 每轮把空闲且无活跃进程的 running 沙盒冻结；`freezeIdle` 在驱动不能冻结时是空操作，绝不把沙盒 stop 掉 |
| 状态语义 | `Manager.Get` 早返回 paused | 冻结中的沙盒工作区还在、驱动侧仍是 running，不能被一次 `Get` 抹掉 |

已验证：`gosandbox` 全部测试与 `GOOS=linux` 交叉编译通过；`internal/...` 全部测试通过（`internal/api` 108s）；`go vet ./...` 无告警。

未验证（当前开发机是 macOS，无 bwrap、无委派 cgroup）：
- 原子入组与 `Limits.Enforced=true` 的真机读回；
- 真实 `cgroup.freeze` 与 `memory.high` 的效果；
- `--size` 在 bwrap ≥ 0.9 上的实际行为。

这三项需要在「已委派 cgroup 的 Linux 主机」上跑一遍 `host/run_linux_test.go` 与新增的 `cgroup_linux_test.go`。

### 10.2 M1 已交付（2026-11-17）

| 组件 | 文件 | 内容 |
|---|---|---|
| 帧协议 | `gosandbox/proto`（`proto.go`、`codec.go`） | `len u32 | kind u16 | payload`；22 种帧；`Frame`/`ReadFrame`/`Encode`/`Decode`/`Send`；字段与帧长双上限，超限在编码侧就拒绝；payload 缓冲复用 |
| agent | `gosandbox/agent`（`agent.go`、`proc.go`、`fs.go`、`stats.go`、`pty_*.go`、`rlimit_*.go`） | PID 1 读循环 + 单写者输出；进程表（`sync.Mutex`，收尸 goroutine 与读循环都要增删）；PTY（`/dev/ptmx` + `TIOCSPTLCK` + `TIOCSWINSZ`）；任意信号打到进程组；rlimit 先设后恢复（fork 继承）；文件 stat/list/read/write/remove/mkdir/rename；`/proc/net/tcp{,6}` 的 LISTEN 端口；cgroup 用量；入站中继（连沙盒内 `127.0.0.1:port`） |
| 宿主侧 | `gosandbox/box`（`session.go`、`handle.go`、`files.go`、`relay.go`、`launch_linux.go`） | `Session` 持有控制连接；`Handle` 的输出走 `io.Pipe`（背压一路传到沙盒内进程的 `write`）；stdin 按 `InputWindow` 限流；`FsWrite` 的帧与裸字节在同一 `outFrame` 里由单写者写出；`net.Conn` 语义的中继；`socketpair` 传控制通道（不在沙盒内开监听） |
| Driver | `gosandbox/vm/box`（`driver_linux.go`、`driver_other.go`） | 实现 `vm.Driver` + `Interactive` + `Meter` + `Pauser` + `Freezer`；`New` 起一个沙盒跑 `true` 做冒烟测试；非 Linux 上全部方法返回 `vm.ErrUnavailable`（fail-closed） |
| 装配 | `internal/config`、`internal/app` | `sandbox.backend: box`；`Image` 暂仍只支持 `host`（镜像根随 M2）；seccomp 用 `default` 档 |
| 端口 | `host.InnerPolicyJSON` | 让 `vm/box` 能生成 helper 策略 JSON，而不必 import `host/internal/spec` |

已验证：`proto` 往返/边界测试、`agent` 端到端测试（含 `-race`）、`box` 在 Linux 上的测试（`Bwrap: ""` 路径）、`vm/local` 与 `vm/box` 的接口一致性、双平台编译与 `vet`。

未验证（当前开发机是 macOS）：
- `box` 套真实 bwrap 的路径（overlay、`--unshare-*`、PID 1 收尸）；
- `vm/box` 的冒烟测试与 `Exec` 在真实 Linux 上的行为（`box/session_test.go` 会在 Linux 主机上自己跑起来）。

下一步（M2 前置）：在 Linux 上跑一遍 `go test ./box/ ./vm/box/`；把 `internal/api` 的 `Unsupported` 端点按新能力逐个替换（`logs` 需要 M1 的日志落盘，`metrics` 已在 M0 接好）。

### 10.3 M2 已交付（2026-11-17）

| 组件 | 文件 | 内容 |
|---|---|---|
| 镜像引用 | `image/reference.go` | `alpine` → `docker.io/library/alpine:latest`；`@sha256:`；`localhost:5000/x` 的端口不被当 tag；大写与坏段一律拒绝 |
| manifest / config | `image/manifest.go` | 索引与单平台 manifest；mediaType 缺失时按内容推断；schema 1 拒绝；只挑 `linux/<arch>` |
| registry 客户端 | `image/registry.go` | 匿名 → 401 challenge → token → 重试；token 按 host+scope 缓存、scope 缩成 pull；凭据只发给 HTTPS 或 loopback；manifest 与层都按内容算摘要核对；上限保护 |
| blob store | `image/store.go` | 内容寻址；`BlobWriter` 先写 tmp、Commit 时校验摘要再 rename；索引原子写；按引用与摘要两种查找；GC 保留索引与调用方声明的 blob |
| 解包 | `image/unpack.go` | 压平成单一 lower（rootless 建不了 whiteout 设备）；`os.Root` 防逃逸（含符号链接中间段）；膨胀/条目/单文件上限；设备与 FIFO 跳过；setuid 清位；`.done` 标记 |
| zstd | `image/zstd.go` | 可注入钩子：默认明确报 `ErrNoZstd`，不静默跳过层，也不引入新依赖 |
| 拉取 | `image/pull.go` | 解析 → config → 逐层（已存在跳过）→ 写索引 → 可选解包；同引用同平台并发合并；进度回调 |
| 导入 | `image/import.go` | `docker save` 的 tar（manifest.json + config + 层）；层摘要按内容算；越界路径拒绝 |
| 根构造 | `rootfs/rootfs.go`、`probe_linux.go` | overlay / copy / host 三种；探测是真起 bwrap 读探针文件；copy 保留符号链接与权限，跳过设备 |
| 日志 | `box/logfile.go` | Docker `local` 的记录格式（`len|stream|time|data|len`）；16 KiB 切分；10 MiB × 3 轮转；按行数/时间/流过滤；尾部半条记录忽略，长度自相矛盾报 `ErrLogCorrupt` |
| 接线 | `box/session.go`、`box/launch_linux.go`、`vm/box/driver_linux.go`、`vm/box/imagesource.go` | `Config.Lower` 作沙盒根；`--overlay` 不可用时拷贝退路；`ImageSource` 只在沙盒 `Spec.Image` 非 host 时使用；没有来源时明确报 `ErrUnavailable` 而不退到宿主根 |
| 配置 | `internal/config`、`internal/app` | `sandbox.images_dir`；配置后自动接上 `image.Puller` 作为 `ImageSource` |

已验证：`image`（引用 / manifest / store / 解包安全 / 端到端拉取打本地假 registry / 导入 / 并发合并 / 摘要不符拒绝）、`rootfs`、`box`（日志）、`vm/box`（镜像解析与路径约束）全部测试通过，含 `-race`；双平台编译与 `go vet` 干净。

未验证（开发机 macOS）：
- 真实 registry 拉取（假 registry 已覆盖协议路径，但没打过 Docker Hub / ghcr）；
- 真实 bwrap 的 `--overlay` 与拷贝退路；
- 大镜像（>1 GB）的解包耗时与磁盘占用。

下一步（M3）：
1. 把 `image` 接到 CLI/API：`pull`、`images`、`rmi` 三个动作；
2. `box/logfile` 接 `GET /sandboxes/{id}/logs`（解锁 E2B `logs`）；
3. agent 的 `FsWatch`（inotify）解锁 envd `WatchDir`；
4. 平台反向代理接 `DialPort`（解锁 `getHost(port)`）。

### 10.4 M3 已交付（2026-11-17）

| 能力 | 落点 | 内容 |
|---|---|---|
| E2B `logs` | `box/log.go`、`vm/box/capabilities.go`、`internal/sandbox/logs.go`、`internal/api/sandbox_logs_handler.go` | 进程输出在 `Handle.deliver` 里同时落盘与转发；`GET /e2b/sandboxes/{id}/logs` 支持 `lines` / `since` / `stream` / `follow=1`（SSE：先回放历史，再 500ms 轮询增量）；沙盒 Stop 后仍能读（日志已落盘） |
| `getHost(port)` | `internal/api/sandbox_logs_handler.go`（`HostProxy`）、`vm.PortDialer` | `POST /e2b/sandboxes/{id}/host/{port}` 经控制通道开中继，把请求转发进沙盒内 `127.0.0.1:port`；沙盒不需要网络栈，宿主也不需要为它开监听 |
| envd `WatchDir` 等四个端点 | `agent/watch_linux.go`、`proto`（`Watch`/`Unwatch`/`WatchEvent`）、`box/watch.go`、`internal/api/e2b_envd_watch.go`、`internal/e2bproto/watch.go` | inotify；200ms 成批合并、按路径去重；递归时把运行中新建的子目录也登记；进程内订阅表；`GetWatcherEvents` 不阻塞（空数组表示暂时没有） |
| 镜像管理 | `internal/api/sandbox_images_handler.go` | `GET /e2b/ext/images`、`POST /e2b/ext/images/pull`（同步拉取 + 解包）、`DELETE /e2b/ext/images`；错误按「引用错 / 需登录 / 平台不支持 / 完整性」分开映射 |

同时修掉的两个既有问题：
- `internal/sandbox.Manager.Get` 会把 `paused` 覆盖成驱动状态，暂停一读就没了（M0 已修，M3 补了测试）；
- `logs` / `metrics` / `host` 之前都用 `cp.Any` 注册，`Any` 会给每个 HTTP 方法各登记一条路由；改成显式的 GET/POST。

已验证：`internal/api` 全量通过（含 logs 的 plain/SSE/501、host 的代理与 400/501、watch 的四个端点、镜像的 501/400/404/列表）；`gosandbox` 的 `box`（日志回放、递归/非递归监听、事件类型）与 `vm/box` 双平台编译通过。

未验证（macOS 开发机）：
- 真实 inotify 在容器密集写入下的合并效果与丢事件率；
- SSE 在反向代理后的行为（是否被缓冲）；
- 真实 registry 的 `pull`。

### 10.5 M3 续：快照/fork 与隧道模式（2026-11-17）

**快照 / fork**（新包 `gosandbox/snapshot`）

| 落点 | 内容 |
|---|---|
| `snapshot/multidir.go` | 一个归档装两棵目录（`upper/` 与 `ws/`），前缀分派；解包只替换归档里出现的前缀 |
| `snapshot/paths.go` | 条目名规范化：绝对路径与 `..` 一律拒绝 |
| `snapshot/snapshot.go` | 内容寻址（`<root>/snapshots/blobs/<sha256>.tar.gz`）+ `index.json`；同内容共用一份 blob，按引用计数删 |
| `vm/box/snapshots.go` | `vm.Snapshotter` 实现；fork 写 `<box>/meta.json` 记血缘；目标有会话时拒绝 |
| `internal/api/sandbox_snapshot_handler.go` | `POST/GET /sandboxes/{id}/snapshots`、`DELETE /snapshots/{id}`、`POST /sandboxes/{id}/fork`、`POST /snapshots/{id}/fork` |

三条语义边界（响应里 `processes=false` 也在说这件事）：
1. **只保存文件系统**：可写层 + 工作区。`work/`、`logs/`、`rootfs/` 不进快照——前者是内核挂载工作区，后两者可从镜像重建；
2. **fork 出来的沙盒没有进程**，需要重新启动。内存快照要 CRIU，与 seccomp 冲突，代价不划算；
3. **运行中拍快照不是原子的**：打包期间磁盘可能变化，是「某个瞬间的近似」。不 Stop 沙盒——杀掉用户的后台进程比一点不一致代价更大。

**隧道模式**（把 `host` 从「一次请求一次响应」改成全双工）

| 落点 | 内容 |
|---|---|
| `proto` | 新增 `KindRelayHalfClose` + `Msg.Flags`（1=关写，2=关读） |
| `box/relay.go` | `Write` 分块、`CloseWrite`/`CloseRead`、`deliverEOF`/`abort`；写后读、读后写都合法 |
| `agent/stats.go` | `relayHalfClose`：宿主说「发完了」就对沙盒内连接做 `CloseWrite`（TCP FIN），沙盒内服务端据此结束读请求 |
| `internal/api` | `HostProxy` 请求体流式转发 + 发完半关闭；响应逐块 `Flush` |

这是 keep-alive 与流式响应能工作的前提：旧实现读完响应就关连接，HTTP/1.1 服务端永远等不到请求体结束。

**已验证**：`snapshot` 全量（往返、去重、无 upper 的恢复、越界拒绝、空沙盒拒绝、索引持久化）；`box` 的隧道语义（半关闭后仍可读、1 MiB 全双工、abort、`CloseWrite` 之后写失败、真实 TCP 上的 FIN → 回写）；`internal/api` 全量（快照的 201/200/204/404/501、fork 的 `Start=false`、隧道的代理与 400/501）。

**未验证**（macOS 开发机）：沙盒内真实的 HTTP 服务在 keep-alive 下的表现、`nc` 的 stream 行为、大镜像 fork 的实际耗时。

M3 剩余（唯一）：**模板**——把「镜像 + 预装内容」固化成一个可复用的创建来源。它与快照共用归档层，缺的是版本与多租户可见性。

---

### 10.6 M4 已交付：受控网络（2026-11-17）

**先说这一节最重要的一行**：加这个功能之前，沙盒的 bwrap 参数里**没有 `--unshare-net`**——
也就是说沙盒共享宿主网络栈，能直接访问云元数据 `169.254.169.254` 与整个私网。这是 SSRF
的经典落点，属于必须先堵住再谈放行的问题。

| 能力 | 落点 | 内容 |
|---|---|---|
| 网络隔离 | `box/launch_linux.go` | 默认 `--unshare-net`：netns 里只有 `lo` |
| lo 拉起 | `agent/loopback_linux.go` | 隔离后 `lo` 是 DOWN 的；不拉起它，`getHost(port)` 与白名单 DNS 全部失效。用 ioctl 置 `IFF_UP`（userns 内的 `CAP_NET_ADMIN` 够用），失败只警告 |
| 出网（egress） | `netx/pasta_linux.go` | pasta + `--config-net --no-map-gw`；`--no-map-gw` 是关键一条：不把宿主 `127.0.0.1` 映射进沙盒 |
| 私网拦截 | `netx/netx.go` | `CheckAllowed` 拦 169.254/16、100.64/10、10/8、172.16/12、192.168/16、127/8、0/8、fc00::/7、fe80::/10、::/128、组播。元数据地址有独立开关（`AllowMetadata`），不与 `AllowPrivate` 混用 |
| 白名单 DNS | `netx/dns.go` | 最小 DNS 代理（UDP+TCP）：只解析白名单域名，其余回 NXDOMAIN；带 60s 缓存让解析结果稳定 |
| 磁盘限额 | `vm/box/diskuse_linux.go` | 宿主侧 du（`upper` + `ws`，10s 缓存）；超限**冻结**而不是杀，用户还能导出文件 |
| 配置 | `internal/config`、`internal/app` | `sandbox.network`（none/egress/egress-allowlist）、`network_allow`、`disk_limit_mib`；空白名单直接启动报错 |

三个设计取舍：

1. **磁盘用量在宿主算，不在 agent 算**。agent 在沙盒里看到的是 overlay 合并视图，分不出
   「镜像本来就有」与「用户写入」；只有宿主知道 `<box>/upper` 与 `<box>/ws` 各多大。
2. **超限冻结而不是杀**。杀掉之后日志、文件、现场都没了；冻结之后用户还能 `pause`、
   读日志、快照导出。与 kiri `usage.rs` 的语义一致。
3. **协议短帧兼容**。`Stats` 新增 `DiskUsedBytes` 时，解码端把「尾部字段缺失」当 0 处理，
   这样不必升 `Version`，两侧可以不同步上线。

**已验证**：`netx` 全量（13 个用例：每个必须拦的网段各一条、边界地址、通配语义
`*.example.com` 不覆盖父域、`none` 档全拒、`AllowPrivate` 不豁免元数据、NXDOMAIN 改写、
畸形查询不 panic、白名单外不打上游）；`internal/config`（三档校验、空白名单报错、
负数限额报错）；`gosandbox` 双平台编译 + vet + 全量测试。

**未验证**（macOS 开发机）：
- pasta 真实出网（需要 Linux + passt）；
- `--unshare-net` 后 `lo` 拉起是否在各类 bwrap 版本下都成立；
- 白名单 DNS 在真实解析场景下的表现（上游可达性、大应答走 TCP 的重试）。

**M4 剩余**：
- **nft set 写入**：白名单 DNS 解出地址后要写进 netns 内的 nft set，才能让「已经解析过的
  域名」在该 TTL 内可连。现在只有代理层，没有这条链路——也就是 `egress-allowlist`
  目前实际等价于「DNS 能解析但连不通」，需要在 Linux 上补完；
- **detach supervisor**（原计划 19）：服务重启不丢沙盒，属于 P3，未开始。

---

### 10.7 M5 已交付（2026-11-17）

**科学划分的三项**：先修「静默的权限差异」，再做「活过重启」，最后做「VM 传输层」。

#### 20. 多段 uid 映射（先修一个真 bug）

发现的问题：协议里 `Msg.User` 从 M1 起就有，**但 agent 从没读过它**。
也就是说 `Exec(ExecRequest{User: "999:999"})` 实际是以沙盒内 uid 0 运行——
用户以为降了权，其实没降。这比报错危险，因为它静默。

| 落点 | 内容 |
|---|---|
| `agent/user.go` | `ResolveID`（数字 / 用户名 / `uid:gid`）、`credentialFor`、`UserMapping.CanSwitchTo` |
| `agent/proc.go` | 启动子进程前解析凭证并设 `SysProcAttr.Credential`；无法映射时退出码 126 + 说明 |
| `box/subid_linux.go` | 探测 `/etc/subuid` + `newuidmap`；生成 `--uidmap`/`--gidmap` 计划（沙盒内 1000 起，总量上限 4096） |
| `proto` | `Exited` 帧增加可选 `Err` 说明（短帧兼容，不升 Version） |

关键取舍：**映射区间从沙盒内 1000 起**。镜像里的服务用户（999、1000、33）大多落在这个量级；
同时避开 1..999 的系统用户段，那里保持未映射可以让越权尝试直接失败。总量截到 4096：
内核映射表是红黑树，六万多条会拖慢每次 uid 查找。

#### 19. detach supervisor（活过重启）

问题：控制通道是 socketpair，一端在服务进程里；服务一退出沙盒就全没了。

解法：把「持有 socketpair」挪到独立进程。

```
服务进程 ──unix socket──► supervisor ──socketpair──► bwrap + agent
```

| 落点 | 内容 |
|---|---|
| `box/supervise_linux.go` | supervisor 主体、重连接管、双向 pump |
| `box/launch_linux.go` | `Config.Supervise` 开启时走 supervisor 路径；已有 socket 则直接接管 |
| `cmd/main_linux.go` | 新增 `--supervisor` 模式（与 agent 共用同一个二进制） |
| `internal/config`、`internal/app` | `sandbox.supervise` |

代价写明在代码里：**断开那一刻 agent 正在发但还没被读走的帧会丢**。不缓冲是刻意的——
缓冲会把 supervisor 变成内存黑洞，而调用方本来就有重试。

#### 21. macOS VM 后端（做了可复用的那一层）

完整实现需要 cgo + `Code-Hex/vz` + 可启动的 VM 镜像，两样都不能在无网络环境里补齐。
所以先交付**与平台无关的传输层**——这也是 VM 后端真正需要的基础：

| 落点 | 内容 |
|---|---|
| `box/transport.go` | `Transport` 接口（io.ReadWriteCloser）+ 可选 `Deadliner`/`Pid`；不再到处写 `net.Conn` |
| `box/vsock.go` | 端口约定（1024 起）、CID 常量、`DialVSock` 带重试（guest 起 agent 与宿主拨号是并发的） |
| `box/vsock_linux.go` | Linux AF_VSOCK 参考实现（可测试：不需要 macOS 就能验证拨号层） |
| `box/vsock_other.go` | 非 Linux 明确报 `ErrVSockUnsupported`，不做半成品 |

为什么 Linux 实现也有价值：它让「拨号 → 传输 → 协议栈」这条链路可以在一台机器上验证，
而不是等 macOS VM 就绪才发现传输层有问题。

**已验证**：`agent`（uid 解析、单/多段映射判定、区间边界、`credentialFor` 的拒绝路径、
映射规格解析）；`box`（supervisor 的参数解析、vsock 端口约定、subid 映射计划与上下限）；
双平台编译 + vet + 全量测试。

**未验证**（macOS 开发机 / 无 VM 环境）：
- 多段映射在真实 bwrap + subuid 下的启动；
- supervisor 的重连（需要 Linux 上跑真实 helper）；
- vsock 真实拨号。

**M5 剩余**：VM 本体（`Code-Hex/vz` + cgo + 镜像），以及白名单的 nft set 链路。

---

## 11. 容量与性能目标（Linux，单机）

| 指标 | 目标 | 依据 |
|---|---|---|
| 热 exec（agent 已在） | p50 < 1ms | 一帧往返 + fork/exec |
| 冷启动（overlay，镜像已解包） | p50 < 100ms | bwrap ~2.3ms（README 实测）+ overlay 挂载 + agent 启动 |
| 冷启动（copy 快照器） | 与镜像大小相关，200MB 镜像 < 2s | reflink 可降到百毫秒级 |
| 恢复（thaw） | < 10ms | cgroup.freeze |
| 空闲沙盒内存 | < 8 MiB（agent RSS） | Go 静态二进制，`GOGC` 调低、`GOMEMLIMIT` 约束 |
| 冻结沙盒 CPU | 0 | — |
| 单机密度 | 500 个冻结沙盒 / 50 个活跃沙盒（16C64G） | 受 memory.max 总和与磁盘约束 |

---

## 12. 风险与对策

| 风险 | 影响 | 对策 |
|---|---|---|
| bwrap < 0.10（如 Ubuntu 24.04 自带 0.9） | 无 overlay | copy 快照器回退；部署文档给出静态 bwrap 构建方式；`Capabilities` 标明 |
| Ubuntu AppArmor 限制非特权 userns | 全部不可用 | 沿用现有探测与错误指引；提供 AppArmor profile 片段而不是让用户改 sysctl |
| cgroup 未委派 | 限额、冻结、指标不可用 | 启动日志与 `Capabilities` 明示；pause 降级为 stop 并告知调用方 |
| 共享内核漏洞 | 沙盒逃逸 | seccomp default 档砍掉大部分攻击面；多租户不可信场景等 M5 VM 后端 |
| agent 崩溃（PID 1 退出） | 整个沙盒结束 | agent 极简、无第三方依赖；崩溃即 stopped，upper/ws 保留可冷启动 |
| 镜像解包被构造攻击 | 写出 lower 目录 | `os.Root` + 六条防护 + 模糊测试（kiri 的测试用例逐条移植） |
| 单段 uid 映射 | 部分镜像（以非 root 用户运行的服务）启动失败 | 文档列为已知限制；M5 |
| 快照冻结窗口 | upper 大时卡顿 | 显示进度；超过阈值拒绝快照并提示清理 |

---

## 13. 结论

- **定位**：gobox 是「**rootless、每沙盒一个 agent 的轻量容器**」，不是 Docker 替代品；对外仍是现有 E2B API，对内仍是 `vm.Driver`。
- **最关键的一步是 agent（M1）**：它同时带来后台进程常驻、热 exec、沙盒视角文件操作、真暂停、端口中继，并且是以后上 VM 的唯一迁移点。镜像（M2）反而是相对机械的工作。
- **从 kiri 实际搬过来的只有 6 样**：agent 当 PID 1、帧协议 + 信用窗口、层解包防护、seccomp 默认表、`CLONE_INTO_CGROUP`、logfile 格式。其余（Docker API、网络栈、BuildKit、NFS、域名、UI）都不需要。
- **M0 现在就能做**，不依赖任何新子系统：修 cgroup 入组竞态、补 freeze/stats、加 masked paths。这一步就能让 E2B 的 `pause` 与 `metrics` 从 Unsupported 变成真实能力。

---

## 附录 A：kiri 数据目录布局

```
~/.kiri/                          (KIRI_DATA_DIR)
  run/docker.sock                 0600（KIRI_SOCKET_GROUP 时 0660 root:kiri）
  containers.json                 容器记录（tmp+rename 原子写）
  volumes.json                    命名卷
  networks.json                   网络
  images.json                     镜像索引
  blobs/sha256/<hex>              压缩层与 config
  tmp/                            下载中
  containers/<id>/
    container.log                 Docker local 格式
    upper/ work/ rootfs/          overlayfs
  volumes/<name>/                 卷数据
  tls/                            自建 CA（ca.pem + ca.key）
  proxy.json                      代理设置
  vm.json                         VM CPU/内存
  logs/<part>.log                 daemon/ui/cli 的 JSON 行日志
  logs/guest-console.log          guest 串口（含 oops），1MiB × 3
  stopped                         「窗口 Stop」标记，跨重启保留
```

guest 内（VM 或 Linux 宿主）：

```
/var/lib/kiri/                   0710
  layers/<hex>/                  解包后的层
  layers/<hex>.done              完整性标记
  containers/<id>/               upper/work/rootfs
  volumes/<name>/
  agent.lock                     flock 单实例
  agent.cgroup                   上次用的 cgroup 路径
/run/kiri/<id>/                  hosts / resolv.conf / hostname（tmpfs）
/sys/fs/cgroup/kiri/             容器 cgroup 父
```

## 附录 B：gosandbox 数据布局（现状；gobox 布局见 §5.3）

```
<Root>/<vm-name>/
  ws/                            工作区（0700）
  stopped                        停止标记
```

沙盒内：

```
/run/gosandbox/policy.json       策略（bwrap --file 放入，0400）
```

## 附录 C：关键常量对照

| 项 | kiri | gosandbox |
|---|---|---|
| 帧上限 | 1 MiB | — |
| 输出块 | 64 KiB | — |
| stdin 窗口 | 4 MiB | — |
| 日志单文件 | 10 MiB × 3 | 内存 `MaxOutputBytes` |
| 日志行长 | 16 KiB 切分 | — |
| 请求体上限 | 8 MiB（archive 流式） | — |
| 请求头上限 | 64 KiB | — |
| 事件保留 | 512 条 / 队列 256 | — |
| 空闲休眠 | 60s | — |
| 重启退避 | 100ms → 1min | — |
| 默认 shm | 64 MiB | — |
| 默认 nofile | 1048576 | — |
| Landlock 最低 ABI | — | 3 |
| macOS 采样间隔 | — | 100ms |
| glob 上限 | — | 1024 条 |
| fsview 条目 | 5000 | — |
| fsview 单读 | 512 KiB | — |
| NFS 句柄 | 50000 | — |