# gosandbox 对标 Anthropic Sandbox Runtime 能力差距与改进计划（planv6.3）

> 状态：**执行中**——阶段 0、A、B、C 已实现并通过测试；阶段 D（违规监控）、E 未开始。详见文末「执行记录」。
> 范围声明：**网络隔离整体不在本计划范围**（不做代理式出口、不做域名 allowlist、不做 TLS 终结与凭据注入；网络维持现状：全关，放开只有 `danger-full-access`）。依赖策略：**允许为本计划所需的能力给 `gosandbox/go.mod` 增加第三方库**，所需库逐一列在 §9；阶段 B 实际**未引入任何第三方库**（见 §9.2 与 §15）。
> 证据基线：
> - Anthropic Sandbox Runtime（下称 **srt**）：`thirdparty/sandbox-runtime`，npm `@anthropic-ai/sandbox-runtime` v0.0.81（Beta Research Preview，Apache-2.0）。
> - SimpleBase **gosandbox**：工作区 HEAD，`host/`（Linux bwrap + Landlock + seccomp；macOS sandbox-exec）+ `vm/`（`Driver` / `local` / `fake`）。
> 所有结论可按 §14 的证据索引回源码核对。

---

## 1. 背景、目标与定位判断

### 1.1 触发点

srt 是 Anthropic 为 Claude Code 开发的进程沙盒运行时：OS 原语（Seatbelt / bwrap）负责文件系统与进程边界，宿主侧用户态负责违规监控等辅助能力。gosandbox 是 SimpleBase 的沙盒执行层，服务两条链路：`gocloudagent` 的本机 shell 工具（`gosandbox/host`）与服务端云沙盒（`vm/local`）。

排除网络后的对标结论：**gosandbox 在内核原语纵深（Landlock 叠加、io_uring/ptrace 拦截、cgroup 资源限额）上不输 srt，部分更强；差距集中在「写侧保护面的覆盖广度」「读侧模式表达力（glob）」「违规可观测」三处。** 其中没有哪一项是“沙盒被轻易击穿”级别的漏洞，属于加固与可用性改进。

### 1.2 定位差异（决定“哪些能力不该抄”）

| 维度 | srt | gosandbox |
|---|---|---|
| 形态 | 常驻宿主进程（Node `SandboxManager`）+ CLI `srt`，包装任意本地进程 | 库模块：每条命令一个短命沙盒进程，无常驻会话 |
| 文件限制实现 | Linux 纯挂载表（无 Landlock），祖先 pin + tmpfs 掩蔽 | bwrap 挂载表 + Landlock 双层（有意纵深，`host/inner/inner_linux.go:55-56`） |
| 资源限制 | **没有**（全 src 搜 `rlimit\|cgroup` 零命中） | cgroup v2（memory/cpu/pids，尽力而为，如实报 `Enforced`） |
| 工作区威胁模型 | cwd 常是用户 HOME / 真实项目；强制 deny 表保护的是**可写区内部**会被宿主侧程序（git、IDE、shell）之后执行的文件 | 本机档：cwd 是用户项目；云沙盒：工作区是服务端私有目录，沙盒内文件只在沙盒内被执行 |
| 失败语义 | fail-closed | fail-closed（隔离建立失败绝不裸跑） |
| 消费者 | Claude Code（人 + 模型共用终端） | SimpleBase 云沙盒 API + gocloudagent 工具（纯 agent） |

**一个必须先说清的前提**：srt 的强制 deny 表只在“被允许写的目录内部”才有意义。gosandbox 的根是 `--ro-bind / /`，`~/.bashrc` 这类 HOME 下的文件本来就不可写，**不属于缺口**，除非用户把 HOME 或其祖先配成 `writable_roots`（见 §3.1）。

### 1.3 分析范围

- srt：`src/sandbox/`（sandbox-manager / sandbox-schemas / sandbox-utils / macos-sandbox-utils / linux-sandbox-utils / glob-pattern / read-deny-glob / sandbox-violation-store / linux-violation-monitor）、`vendor/seccomp-src/apply-seccomp.c`、README 的文件系统、强制 deny、违规监控与局限章节。**网络栈（http/socks/mux/tls/mitm/decider/credential-*）与 Windows 后端不在范围内。**
- gosandbox：`host/`（policy / probe / bwrap_args / run_linux / start_linux / inner / seatbelt / cgroup）、`vm/`（driver / local / fake / logged）、`cmd/gosandbox-inner/`。
- 关联面：`gocloudagent/cli/shell_isolation.go`（本机档策略入口）、`gocloudagent/tools/tools.go`（返回信封）、`internal/sandbox`（云沙盒业务层）。

### 1.4 结论摘要（一页纸）

**27 项能力对照（不含已排除的网络与 Windows 项）：✅ 9 / ⚠️ 8 / ❌ 7 / ⛔ 3。**

| 编号 | 缺口 | 真实影响（已按源码校准） | 优先级 |
|---|---|---|---|
| P0-1 | 写侧强制 deny 表覆盖不全，且 Linux 对“尚不存在的路径”直接跳过 | `ReadOnlySubpaths` 默认只有 `.git`、`.gocloudagent`。`.gitmodules` / `.vscode` / `.idea` / `.mcp.json` 无保护；`.git` 尚不存在时（`git init`）Linux 跳过（`bwrap_args.go:93`），沙盒内可写出 `.git/hooks`，之后宿主侧 git 执行。**macOS 因 `subpath` 本身是路径语义，已覆盖不存在路径，Linux 与 macOS 行为不一致** | P0 |
| P0-2 | `deny_read` 只有字面路径，无 glob | `**/.env`、`*.pem` 这类“任意深度的密钥文件”无法表达，是现实的密钥读取面 | P0 |
| P0-3 | 默认把 `.git` 整个只读，沙盒内 `git commit` 必失败（README:56 自述） | 与 srt 的“只保护 `.git/hooks` 与 `.git/config`”相比，gosandbox 为了安全牺牲了可用性。**D1 已决：改**——P0-1 落地后把默认改为 `[".gocloudagent"]`，`.git` 内只保护 hooks / config / info/attributes / 指针文件，让 agent 能提交 | P0（依赖 P0-1 先行） |
| P1-1 | 无违规监控与反馈闭环 | 沙盒拒绝对 agent 是黑盒（只有 EPERM），模型反复撞墙。macOS 可低成本做，Linux 无低成本方案 | P1 |
| P1-2 | 无祖先 pin | **价值未验证**：gosandbox 的根是 `--ro-bind`，被保护路径本身已是挂载点；祖先被 `mv` 后保护是否失效需先写复现用例，复现才做 | P1（有条件） |
| P1-3 | 超大挂载表无 `--args fd` 通道 | glob 展开后挂载数量上升，才会碰到 bwrap 参数上限（srt 9000，`linux-sandbox-utils.ts:863`） | P1（依赖 P0-2） |

P2 另列 6 项（两段式进程布局、嵌套 userns 双保险、错误码体系、argv 快照与平台矩阵测试、性能基线、Linux 违规观察），按需排期。

**不采纳**（§6）：Windows 后端、srt CLI 产品面、弱化降级档、去 Landlock 化、默认拒绝式 seccomp、动态策略更新；网络相关整体范围外。

---

## 2. 能力对照矩阵

判定列描述 **gosandbox 现状**。

### 2.1 文件系统隔离（10 项：✅5 / ⚠️3 / ❌2）

| # | 能力 | srt 实现（证据） | gosandbox 现状（证据） | 判定 |
|---|---|---|---|---|
| 1 | 三档模式 | 同语义 | `host/policy.go:26-30`（与 Codex sandbox_mode 对齐） | ✅ |
| 2 | 写 allow-only + 区内 deny 洞 | `FsWriteRestrictionConfig`（`sandbox-schemas.ts:66-71`） | `WritableRoots` + `ReadOnlySubpaths`（`policy.go:47,91-96`） | ✅ |
| 3 | 读侧 deny-then-allow（`allowRead` 在拒绝区挖洞，most-specific-wins） | `sandbox-schemas.ts:1-48`、README:117-120 | allow-then-deny，后写 deny 覆盖；macOS 读侧全局放开 `file-read*`（`seatbelt.go:23`）；`vm/local` 以 `ProtectDirs` 补救 | ⚠️ 语义不同，**本计划不改** |
| 4 | glob 模式（`**/.env`、方括号字面量容错、`literal:true`） | `glob-pattern.ts:8-23,48-76`、README:581-584 | 无：全部字面路径，`DeniesRead` 做前缀匹配（`policy.go:163-175`） | ❌ |
| 5 | 强制 deny 默认表 | `DANGEROUS_FILES` / `DANGEROUS_DIRECTORIES`（`sandbox-utils.ts:21-46`）、README:1142-1156 | 仅 `.git`、`.gocloudagent` 整体只读（`policy.go:47`） | ⚠️ |
| 6 | 祖先 pin | `linux-sandbox-utils.ts:1886-1912`；macOS `generateMoveBlockingRules`（`macos-sandbox-utils.ts:932-961`） | 无 | ❌（价值待复现） |
| 7 | 不存在路径也被保护 | Linux 造挂载点（`linux-sandbox-utils.ts:661-665,1141-1149`）；macOS glob 天然覆盖 | **Linux 跳过**（`bwrap_args.go:93,102`）；macOS `subpath` 不判存在（`seatbelt.go:60-73`），会让 `git init` 失败 | ⛔ **D7 已决：不抄**，统一为“不存在就不保护” |
| 8 | Landlock 纵深 | **无**（全仓 `landlock` 零命中） | `inner_linux.go:57-71`，严格 V3 不降级（:110-115） | ✅ 独有优势，保留 |
| 9 | env 最小集合（不继承宿主变量） | `unsetEnvVars` / `setEnvVars` | `vm/local` `baseEnv` 5 个变量 + 调用方 Env（`local.go:276-284`，测试断言服务密钥不泄漏） | ✅ 更严 |
| 10 | `deny_read` 联动父进程文件工具 | 凭据 deny 表 | `host.DeniesRead` 同步约束父进程文件工具（`policy.go:161-175`） | ✅ |

### 2.2 进程与内核加固（7 项：✅3 / ⚠️1 / ❌1 / ⛔2）

| # | 能力 | srt 实现（证据） | gosandbox 现状（证据） | 判定 |
|---|---|---|---|---|
| 1 | io_uring 拦截 | `apply-seccomp.c` | `inner_linux.go:89-90` | ✅ |
| 2 | ptrace / `process_vm_*` 防护 | 两段式布局 + `PR_SET_DUMPABLE=0`（`apply-seccomp.c:781-788`） | seccomp 直接拒 syscall（`inner_linux.go:90`） | ✅ |
| 3 | 两段式进程布局（bwrap 内再造嵌套 user+PID ns，init/helper 不可见、不可 ptrace） | `apply-seccomp.c:11-30` | 无：bwrap init 与 helper 对沙盒进程可见 | ❌（P2） |
| 4 | 嵌套 userns 双保险 | `apply-seccomp.c:32-39`、`linux-sandbox-utils.ts:3851-3869` | 只有 bwrap `--disable-userns`（bwrap≥0.8.0，`bwrap_args.go:114-116`） | ⚠️ |
| 5 | 默认拒绝式 seccomp 白名单 | srt 也没有（同为默认允许 + 窄黑名单） | `DefaultAction: ActionAllow` + 黑名单（`inner_linux.go:87-95`） | ⛔ 对等，不抄 |
| 6 | 资源限额 | **没有** | cgroup v2 `memory.max` / `cpu.max` / `pids.max`，未委派时如实报 `Enforced=false`（`cgroup_linux.go:37-99`） | ✅ 独有优势 |
| 7 | 弱化降级（`enableWeakerNestedSandbox`） | README:591,1278 | fail-closed，探测不过即 `ErrIsolationUnavailable` | ⛔ 原则分歧，不抄 |

### 2.3 违规监控与可观测（4 项：全 ❌）

| # | 能力 | srt 实现（证据） | gosandbox 现状 | 判定 |
|---|---|---|---|---|
| 1 | macOS 违规日志 tap（`log stream` + 每条 deny 带 logTag） | `macos-sandbox-utils.ts:1081-1083,1627-1650` | Seatbelt profile 为 `(deny default)`，无 `with message`，无任何日志采集（`seatbelt.go:14-29`） | ❌ |
| 2 | Linux 违规观察（seccomp USER_NOTIF → unix socket → monitor，明确“只是诊断提示，不作策略依据”） | `linux-violation-monitor.ts:20-105` | 无 | ❌（P2） |
| 3 | 命令归因编码（同一 token 复用于日志标签与记录） | `linux-sandbox-utils.ts:3603-3609`、`macos-sandbox-utils.ts:1639-1659` | 无 | ❌ |
| 4 | 违规 → agent 提示闭环 | Claude Code 集成层 | 无：agent 只看到 EPERM | ❌ |

### 2.4 策略模型与工程化（6 项：✅1 / ⚠️4 / ⛔1）

| # | 能力 | srt 实现（证据） | gosandbox 现状 | 判定 |
|---|---|---|---|---|
| 1 | 策略静态校验 | `config-validation.test.ts` | `policy.go:116-120,135-159` + `policy_test.go` 表驱动 | ✅ |
| 2 | 错误可编程化（`LinuxSandboxProfileError.code`） | README:1140 | `vm/errors.go` 有后端错误分类，无面向 agent 的 code | ⚠️ |
| 3 | 大参数通道（O_TMPFILE + `--args fd`） | `linux-sandbox-utils.ts:862-863,1028-1067` | 策略 JSON 已走 fd 3（`bwrap_args.go:118-119`），挂载表仍走 argv | ⚠️ |
| 4 | bwrap argv 断言 helper + 平台矩阵 + 属性测试 | `test/bwrap-argv.ts`、`glob-walk.property.test.ts` | 有真集成测试（自动 Skip）与 profile 文本断言，无 argv 快照、无矩阵 | ⚠️ |
| 5 | 性能基线 | n/a | bwrap+helper 组合未测（`README.md:82` 自认） | ⚠️ |
| 6 | 动态策略更新 `updateConfig` | `SandboxManager` | 每命令短命进程，无会话 | ⛔ 模型不同 |

---

## 3. P0 缺口详细设计

### 3.1 P0-1 写侧强制 deny 表（**不存在的路径不保护**，D7 已决）

**威胁校准（相对 v1 草稿的修正）**：

- 不再把 `~/.bashrc` 当缺口：根只读，HOME 下文件不可写。仅当某个 `writable_root` **覆盖了用户 HOME**（`gocloudagent/cli/shell_isolation.go:21` 允许用户配置）时，才需要 rc 文件保护。
- 真实缺口是**可写区内部**会被宿主侧程序之后执行/加载的文件。云沙盒的工作区只在沙盒内被执行，价值有限；**本机档（gocloudagent 在用户真实项目上跑）价值最大**。

**默认表（对齐 srt `sandbox-utils.ts:21-46`，按 gosandbox 场景裁剪）**：

| 层 | 路径（相对 Workspace） | 理由 |
|---|---|---|
| 恒生效 | `.git/hooks`、`.git/config`、`.git/info/attributes`、`.gitmodules`、`.gocloudagent`、`.mcp.json` | 宿主侧 git / MCP 客户端会执行或读取 |
| 恒生效 | `.vscode`、`.idea` | IDE 打开项目即可触发任务/运行配置 |
| 条件生效 | `~/.bashrc`、`~/.zshrc`、`~/.profile`、`~/.bash_profile`、`~/.zprofile`、`~/.gitconfig` | 仅当某个可写根覆盖 `os.UserHomeDir()` 时追加 |

`.git` 的处理见 §3.3。表内容放 `host/policy.go` 导出变量；`Policy.ForceDenyWrite`（默认 `true`；指针或反向命名，避免零值歧义）控制，显式关闭时 `Result` 附 warning。

**与现有 `ReadOnlySubpaths` 的关系**：强制表是叠加层，`ReadOnlySubpaths` 保持用户可配；两者取并集后生成挂载/规则，去重。

**不存在路径的语义（D7 已决：不存在就不保护，两平台统一）**：

- 实测依据（阶段 A，macOS 真机）：对不存在的 `.git/hooks` 施加 `deny file-write*` 后，沙盒内 `git init` **直接失败**（`.git/hooks/: Operation not permitted`，exit 1，并留下残缺的 `.git`）；对整个 `.git` 施加则 `.git` 都建不出来。这与 D1「让 agent 能提交」直接冲突，因此**不对尚不存在的路径做保护**。
- Linux：保持现状（`bwrap_args.go` 对不存在路径跳过，因为 `--ro-bind` 到不存在路径会令 bwrap 启动失败）。**不引入**宿主占位挂载点与生命周期模块（原计划的“临时挂载点生命周期模块”整体删除，风险 R1 随之消除）。
- macOS：现状对 `ReadOnlySubpaths` 不判存在，会把不存在的路径也 deny，与 Linux 不一致，且正是 `git init` 失败的原因。**改为与 Linux 一致：只对生成 profile 时已存在的路径生成 deny 规则。**
- 残余风险（写进 README，不假装已封死）：同一条命令里先 `git init` 再写入 `.git/hooks/*`，hook 会落盘，用户之后在该目录运行 git 时执行。缓解：`init` 之后的**下一条**命令起 `.git/hooks` 已存在，即受保护；单条命令内的窗口无法在不破坏 `git init` 的前提下关闭。

**验收要点**：

1. 工作区已有 `.git` 时，沙盒内写 `.git/hooks/pre-commit`、`.git/config` 失败（Linux 与 macOS 一致）。
2. 工作区无 `.git` 时，沙盒内 `git init` 成功且之后 `git commit` 成功；`init` 之后的**下一条**命令写 `.git/hooks/x` 失败。
3. 写已存在的 `.gitmodules`、`.vscode/*`、`.mcp.json` 失败；写普通文件成功。
4. 可写根覆盖 HOME 时，`echo x >> ~/.bashrc` 失败；不覆盖时该条目不生成（argv 快照断言）。
5. `ForceDenyWrite=false` 时行为回到现状，`Result` 带 warning。

### 3.2 P0-2 `deny_read` glob 与路径匹配引擎

**目标**：让 `deny_read` 表达“任意深度的密钥文件”，如 `**/.env`、`**/*.pem`、`~/.ssh/**`。

**适用面（保守边界，对齐 srt README:560-563）**：只放开 `DenyRead` 的 glob。`WritableRoots` / `ReadOnlySubpaths` 保持字面量。

**两平台语义**（有意不同，必须写进文档）：

| 平台 | 实现 | 语义 |
|---|---|---|
| Linux | 构建 bwrap 参数时展开为**当下存在**的路径，目录 `--tmpfs`、文件 `--ro-bind /dev/null` | **快照语义**：之后新建的匹配文件不受掩蔽（srt README:562 同样限制） |
| macOS | 把 glob 转成 SBPL `(deny file-read* (regex #"..."))`（对齐 srt `pathFilter`，`macos-sandbox-utils.ts:164-216`） | 覆盖未来新建的文件 |
| 父进程文件工具 | `host.DeniesRead` 用同一套 glob 匹配 | 覆盖未来新建的文件 |

**glob 语义**（对齐 srt `glob-pattern.ts`）：`*` 不跨段，`**` 跨段；**拼不出集合的方括号按字面量处理**，判据是 srt 的字符集解析：`[user-id]`（`r-i` 范围倒置）、`a[b`（无 `]`）、`[]`（空集合）按字面文本，而 `[WIP]` 是合法集合（W / I / P 三选一），**不是**字面量（初稿此处写错，已更正）；配置校验期拒绝结尾分隔符（README:581）；`literal:` 前缀强制字面量（README:584）。无转义字符。

**实现（偏离初稿：不引入 `doublestar`）**：把 srt 的 `glob-pattern.ts` 解析器移植成 Go（`host/glob.go`），一份 token 解析同时供三处使用：macOS 的 SBPL 正则、`DeniesRead` 的匹配、Linux 展开时的逐段匹配。理由：srt 的方括号降级规则（空集合、范围倒置、无 `]` 按字面）需要自己实现，套一层第三方库后还要再处理两套引擎并存的漂移（R4）。单一解析器从结构上消除了“regex 与 doublestar 不一致”的可能，因此原计划的一致性属性测试不再需要。连续通配符在解析时合并，避免 SBPL 正则回溯爆炸。

**限额**：展开路径数量上限默认 1024，超限返回 `ErrInvalidPolicy` 风格错误（不静默截断，对齐 fail-closed）；挂载总量逼近 bwrap 参数上限时由 P1-3 的 `--args fd` 承接。

**验收要点**：

1. `**/.env` 在两层嵌套目录下均被掩蔽（Linux）/拒绝（macOS）。
2. 沙盒内新建 `sub/.env`：Linux 不受掩蔽（快照语义，测试明示）；macOS 被拒绝；`DeniesRead` 命中。
3. `[user-id]/x` 这类拼不出集合的方括号被当作字面路径，不报错；`[WIP]` 作为合法字符集处理。
4. 展开 > 1024 条时返回错误而不是半途生效。
5. 现有 `policy_test.go` 的字面路径用例全部保持通过。

### 3.3 P0-3 `.git` 保护粒度：从“整体只读”改为“hooks + config 只读”（**D1 已决：做**）

**现状**：`DefaultReadOnlySubpaths = [".git", ".gocloudagent"]`（`policy.go:47`），整个 `.git` 只读，沙盒内 `git commit` 必失败（`README.md:56` 自述）。

**目标**：默认列表改为 `[".gocloudagent"]`；`.git` 的保护由 P0-1 的强制表接管，只覆盖会被宿主侧执行或改变行为的路径。`.git` 其余部分（objects、refs、index、HEAD）可写，`git commit` / `git checkout` 可用。这正是 srt 的做法（README:1156）。

**前置顺序约束（硬性）**：必须在 P0-1 的强制表（含 `.git/hooks`、`.git/config`、`.git/info/attributes`）**落地并通过测试之后**才能改默认值。否则中间态会出现“`.git` 已放开而 hooks 尚未被单独保护”的安全回退。因此阶段 C 严格排在阶段 A 之后，且两者不得拆成不同发布批次。

**`.git` 内的保护范围（强制表扩充，替代“整个 `.git`”）**：

| 路径 | 理由 |
|---|---|
| `.git/hooks`（目录） | 宿主侧 git 会执行 |
| `.git/config` | `core.fsmonitor` / `core.hooksPath` / `alias.*` / `core.sshCommand` 均可导致宿主侧执行 |
| `.git/info/attributes` | 可配置 `filter.*` 驱动（需配合 config，但属同类向量，从严保护） |
| `.gitmodules`（工作区根，已在表内） | 子模块 URL 与 `update` 命令 |
| `.git` 本身为**文件**（worktree / submodule 的 `gitdir:` 指针） | 改写指针可把后续 git 操作导向任意仓库；对该文件 `--ro-bind` / `deny file-write*` |

`.git/info/exclude`、`packed-refs`、`HEAD`、`index`、`objects/`、`refs/` 保持可写。**已知残余风险（写进 README，不假装已封死）**：可写的 `HEAD`/`refs` 允许 agent 改写分支指向；`objects/` 可写允许植入对象。它们不直接导致宿主侧代码执行，但会影响“用户之后检出/合并到什么”。这是 D1 选择“可 commit”必然接受的代价，与 srt 一致。

**风险与边界**：

- **`.git` 为文件时的解析**：`gitdir: <path>` 指向的真实目录可能在工作区之外（worktree 常见）。强制表要跟随该指针，把**指针指向目录**内的 `hooks`、`config` 也纳入保护；该目录若不在任何可写根内则本就只读，无需处理，需写用例覆盖“指向可写根内 / 外”两种情形。
- **`.git` 不存在时的行为（D7 已决）**：不占位、不保护，`git init` 可正常完成（macOS 真机实测占位会令 `git init` 直接失败，见 §3.1）。`init` 完成后 `.git/hooks`、`.git/config` 即已存在，**下一条命令起**自动受强制表保护。单条命令内“先 init 再写 hook”的窗口是已知残余风险，写进 README。
- **三层默认值的联动（见 §9.1）**：`DefaultReadOnlySubpaths` 的值同时出现在 Go 代码、YAML 示例注释、文档中，必须同批修改，避免文档与行为不一致。
- **行为变更对象**：已部署 gocloudagent 用户、云沙盒调用方。沙盒内从“不能 commit”变为“能 commit”；**不提供回退开关以外的兼容层**——想维持旧行为的操作者显式配置 `read_only_subpaths: [".git", ".gocloudagent"]` 即可（该配置路径现已存在，`shell_isolation.go:23`）。
- 同时保留的显式配置语义不变：`nil` 取新默认、空切片表示不盖（`policy.go:90-96`）。注意：**显式空切片只关闭 `ReadOnlySubpaths`，不关闭强制表**；强制表由 `ForceDenyWrite` 单独控制。

---

## 4. P1 缺口详细设计

### 4.1 P1-1 违规监控与 agent 反馈闭环

**范围收敛**：网络违规已排除，剩余只有文件系统与进程类拒绝。

1. **通用层**：`host` 新增
   ```go
   type Violation struct {
       Kind string // "file_read" | "file_write" | "process" | "other"
       Path string // 尽力而为，可能为空
       Op   string
       At   time.Time
   }
   ```
   `host.Result` 与 `vm.ExecResult` 新增 `Violations []Violation`（上限 32 条，去重聚合）。
2. **macOS（可做）**：
   - Seatbelt 头部改为 `(deny default (with message "gosb:<token>"))`（对齐 srt `macos-sandbox-utils.ts:1083`），每条 deny 带可过滤标签。
   - 命令运行期间 `log stream --predicate '(eventMessage ENDSWITH "gosb:<token>")' --style compact` 采集，命令结束停止并归并进 `Result.Violations`。
   - 噪声过滤清单（srt `:1684-1691` 有参考）。**实施前需在真机验证 `log stream` 权限与延迟**（见风险 R5）。
3. **Linux（本计划不做）**：bwrap/Landlock 无原生违规上报；srt 的 seccomp USER_NOTIF 方案需要 libseccomp（cgo）且把拦截面从“拒绝”改为“通知”，风险面大（srt 自己标注“绝不能作为策略依据”，`linux-violation-monitor.ts:72-75`）。`go-seccomp-bpf` 不支持 notif。Linux 上 `Violations` 恒为空，如实文档化（沿用 `limits=unenforced` 的“如实报告”范式）。
4. **命令归因**：`Result` 带一次性随机 token，日志标签与记录共用，用于关联。
5. **agent 提示闭环**：`gocloudagent/tools/tools.go` 的返回信封（现有 `limits=unenforced`，`:218-222`）增加 `violations` 字段，沿用同一信封格式；云沙盒侧是否透出到 API 响应走 D3。

**边界**：违规路径属元数据可记；绝不记命令输出正文以外的敏感内容。

### 4.2 P1-2 祖先 pin（先复现，再决定做不做）

**为什么不直接做**：srt 的祖先 pin 是为“被保护路径位于可写 bind 之内，其祖先目录可被 `mv`/`rmdir` 破坏保护”设计的。gosandbox 的被保护路径在挂载后**本身就是挂载点**，对它自己 `mv`/`rmdir` 已得 `EBUSY`。需要验证的只是“祖先被搬走后，保护是否对宿主侧后续的同路径访问失效”。

**步骤**：

1. 先写复现用例（真 bwrap）：`mv ws/sub ws/sub2`（`sub` 内含被保护路径）后，检查保护是否仍有效、原路径能否被重新写入。
2. 复现成功 → 对被保护路径的每个已存在祖先（到可写根为止）做 self-bind（`--bind d d`）；macOS 对齐 `generateMoveBlockingRules`（`(deny file-write-unlink ...)` + 可写根 re-allow，`macos-sandbox-utils.ts:932-961`）。
3. 复现不成功 → 关闭本项，在 README 记录“已验证无需祖先 pin”。

### 4.3 P1-3 `--args fd` 大参数通道

glob 展开（P0-2）与强制表占位（P0-1）会增加挂载数量。对齐 srt：参数超阈值时把 bwrap 参数写入 `O_TMPFILE`，用 `bwrap --args <fd>` 读取（`linux-sandbox-utils.ts:1028-1067`）。先量化当前挂载数量分布，再定阈值；若 P0-2 的 1024 上限下总参数仍远低于 bwrap 上限，则本项降为 P2。

---

## 5. P2 缺口详细设计（按需，逐项独立评审）

1. **两段式进程布局**（`apply-seccomp.c:11-30`）：bwrap 内再造嵌套 user+PID ns，使 bwrap init / helper 对沙盒进程不可见、不可 ptrace。gosandbox 已用 seccomp 拒 ptrace，此项是纵深加固，inner 复杂度显著上升。
2. **嵌套 userns 双保险**（`apply-seccomp.c:32-39`）：补 seccomp namespaces 过滤器，需评估是否可行与权限。
3. **错误码体系**：`host` 定义 `ProfileError{Code}`，gocloudagent 按码分支提示（对齐 srt README:1140）。
4. **bwrap argv 快照断言 + 平台矩阵**：仿 `test/bwrap-argv.ts`，把 `bwrap_args.go` 输出做成黄金文件；矩阵含 bwrap 0.8 / 0.10+ 与 Ubuntu AppArmor 环境。**建议随 P0 阶段顺手做**，成本低、能给 P0-1/P0-2 的参数变更兜底。
5. **性能基线**：在有 bwrap 的 Linux 机器上补测“bwrap + gosandbox-inner + /bin/true”，回填 `README.md:75-82`。
6. **Linux 违规观察**：seccomp USER_NOTIF 方案，见 §4.1 第 3 点，默认不做。

---

## 6. 明确不采纳 / 范围外（含理由）

| # | 能力 | 理由 |
|---|---|---|
| 1 | 网络隔离相关全部能力（代理、域名 allowlist、IP 守卫、TLS MITM、凭据哨兵/注回/SigV4 重签、外部 decider、unix socket 放行档） | **用户明确范围外**。网络维持现状：全关，放开只有 `danger-full-access`；`vm.Spec.Network` 保持 `none\|public` 与 local 后端拒绝 `public` 的现状不变 |
| 2 | Windows 后端 | SimpleBase 服务端不支持 Windows；gosandbox 现契约即其他平台 `ErrIsolationUnavailable` |
| 3 | srt CLI 产品面（`srt` 命令、`~/.srt-settings.json`、MCP 包装） | gosandbox 是库模块，不是终端工具 |
| 4 | `enableWeakerNestedSandbox` 类弱化降级 | 与 fail-closed 原则冲突 |
| 5 | 去 Landlock 化（srt 纯挂载表方案） | gosandbox 的 Landlock 严格 V3 双层是有意纵深，保留 |
| 6 | 默认拒绝式 seccomp 白名单 | srt 同样没有；维护与兼容成本远超收益 |
| 7 | 动态策略更新 `updateConfig` | 每命令短命沙盒，策略在每次 `Run/Start` 前构建 |

---

## 7. 决策记录与待决策项

| # | 决策 | 状态 / 选项与建议 |
|---|---|---|
| D1 | 默认把 `.git` 整体只读改为“仅 hooks + config 等只读”（让沙盒内可 `git commit`） | **已决：改。** 实施约束见 §3.3：必须在 P0-1 强制表落地后才改默认值，两者同批发布 |
| D2 | `ForceDenyWrite` 是否暴露给 `gocloudagent` 的 `ShellIsolation` 配置（`shell_isolation.go`） | 建议**不暴露关闭开关**，只在 `host.Policy` 层保留；确有需要再单独评审 |
| D3 | `Violations` 是否透出到云沙盒 API 响应或落系统库 | 建议先只进 `Result` + gocloudagent 信封 + zap 日志；API 透出或入系统库另评审（DDL 需用户手动执行） |
| D4 | macOS glob 转 SBPL regex（覆盖未来文件）与 Linux 展开快照两侧语义是否接受 | 建议接受并文档明示；替代方案是 macOS 也展开快照，语义一致但弱化 macOS |
| D5 | 祖先 pin 是否实施 | 由 §4.2 复现结果决定，不预先承诺 |
| D6 | 引入 `doublestar` 及连带的根模块 / `gocloudagent` 的 `go.mod`、`go.sum` 更新 | **已决：同意，但实施时未使用。** 阶段 B 改为移植 srt 解析器自研（§3.2），三处 `go.mod` / `go.sum` 零改动；授权仍保留给后续确有需要的阶段 |
| D7 | 工作区尚无 `.git` 时，强制 deny 表是否对不存在路径占位保护 | **已决：不保护。** 依据 macOS 真机实测（占位使 `git init` 失败）；Linux/macOS 统一为“只保护已存在路径”，删除挂载点生命周期模块 |

---

## 8. 分阶段实施清单

### 阶段 0：契约冻结（0.5d）

- `Violation` / `ForceDenyWrite` 字段形态、强制表内容（含 `.git` 内的扩充项，见 §3.3）定稿。
- D1、D6 已决，无待确认项。

### 阶段 A：P0-1 写侧强制 deny（3–4d）

1. 强制表与 `Policy` 合并逻辑（纯函数，含 HOME 条件追加，测试先行，1d）。
2. 强制表接入 Linux bwrap 参数与 macOS Seatbelt profile，两平台统一“仅对已存在路径生成规则”（macOS 需补存在性判断，0.5–1d）。
3. argv / profile 快照断言（P2-4 提前，0.5d）。
4. ~~真 git 实测~~ 已完成（macOS）：占位使 `git init` 失败 → D7。Linux 侧待有 bwrap 的机器补测。

### 阶段 B：P0-2 glob（3d）

**已完成**（见 §15）：移植 srt 解析器 → `Validate` 校验 → `DeniesRead` 同步 → Linux 展开与限额 → macOS glob→regex → 两平台端到端用例与配置示例。原“一致性属性测试”因单一解析器而取消。

### 阶段 C：P0-3 `.git` 粒度（1–1.5d，**必须在阶段 A 完成后，与 A 同批发布**）

1. 强制表补 `.git/config`、`.git/info/attributes`、`.git` 指针文件（含 `gitdir:` 跟随），测试先行（0.5d）。
2. 默认值 `DefaultReadOnlySubpaths` 改为 `[".gocloudagent"]`，**同批**更新全部联动点（见 §9.1 联动清单）（0.25d）。
3. 用例：真 git 的 `init` / `add` / `commit` / `checkout` 成功；hooks、config 写入失败；worktree 与 submodule 的 `.git` 文件场景（0.5d）。
4. `gosandbox/README.md:29,56` 与 `docs/automation/sandbox.md:44`、`config.example.yaml:212` 文案同步（0.25d）。

### 阶段 D：P1-1 违规监控（3–4d）

通用 `Violation` 通道 → macOS `with message` + `log stream` 采集与噪声过滤 → 归因 token → gocloudagent 信封 `violations` 字段。

### 阶段 E：P1-2 / P1-3（1–3d，有条件）

祖先 pin 复现（先 0.5d）；`--args fd` 视量化结果。

### 阶段 F：P2（按需，逐项独立评审）

每阶段完成判据：`cd gosandbox && go build ./... && go vet ./... && go test ./...` + `GOOS=linux go vet ./host/... ./cmd/...` 全绿；`cd gocloudagent && GOWORK=off go build ./... && go vet ./... && go test ./...` 与根 `go build ./internal/... ./cmd/...` 通过；真 bwrap / sandbox-exec 用例在有环境处人工触发一轮。

---

## 9. 接口、契约与依赖变更清单

### 9.1 接口与契约

| 位置 | 变更 | 兼容性 |
|---|---|---|
| `gosandbox/host.Policy` | + `ForceDenyWrite`（默认开）；`DenyRead` 允许 glob | 默认开启即**行为变更**（更严）；`DenyRead` 的字面路径语义不变 |
| `gosandbox/host` | + 强制 deny 表、+ glob 校验与匹配（`ExpandGlob`、`MaxGlobMatches`、`ErrGlobTooLarge`）、+ `Violation` | 新增；`Violation` 属阶段 D，未实现 |
| `gosandbox/host.Result` / `vm.ExecResult` | + `Violations []Violation` | 纯新增字段 |
| `gosandbox/host.DefaultReadOnlySubpaths`（`policy.go:47`） | `[.git, .gocloudagent]` → `[.gocloudagent]`（D1 已决） | **默认行为变更**：沙盒内 `git commit` 由失败变可用；想保持旧行为的操作者显式配 `read_only_subpaths: [".git", ".gocloudagent"]` |
| `gocloudagent/tools` 返回信封 | + `violations` | 沿用 `limits=unenforced` 的信封范式 |
| `vm.Spec.Network` | **不变** | 网络范围外 |
| `config.yaml` | **不变** | 本计划不新增配置节 |

边界重申：`gosandbox` 不 import 根模块任何包（含 `internal/...`）；`host` 不 import `vm`；agent 语义（提示文案）留在 `gocloudagent` / `internal/agentbridge`，不下沉到 gosandbox。

**`DefaultReadOnlySubpaths` 变更的联动清单（D1，同批修改，缺一不可）**：

| 位置 | 现状 | 动作 |
|---|---|---|
| `gosandbox/host/policy.go:47` | `[".git", ".gocloudagent"]` | 改默认值 |
| `gosandbox/host/policy_test.go:123-141,181-207` | 以 `/work/ws/.git` 的 `--ro-bind` 断言顺序与覆盖 | 改为以 `.gocloudagent` 与强制表项断言；`TestBwrapArgsOrder` 的“顺序错了会让 .git 变回可写”注释改写 |
| `gosandbox/host/seatbelt_test.go:67,83` | 断言 `R0=.../.git`、`R1=.../.gocloudagent` | 改为新默认 + 强制表项（序号会变，用名称而非序号断言以免再脆） |
| `gosandbox/host/run_linux_test.go:97-102`、`run_darwin_test.go:73-78` | 断言“`.git` 必须只读”（mkdir / touch 在 `.git/hooks`） | 改为断言 `.git/hooks`、`.git/config` 只读，同时新增断言 `.git/refs`、`objects` 可写 |
| `gosandbox/README.md:29,56` | “`.git`、`.gocloudagent` 再盖成只读”；“`git commit` 会失败” | 重写：说明新默认与残余风险（§3.3），删除“要让 agent 提交把 `.git` 去掉”的旧说法 |
| `docs/automation/sandbox.md:44` | “`.git` 与 `.gocloudagent` 目录在工作区内是只读的” | 同步 |
| `config.example.yaml:212` | 注释“省略则取默认 [.git, .gocloudagent]” | 同步为 `[.gocloudagent]` |
| `internal/config/config.go:252`、`yaml.go:198` | `ReadOnlySubpaths` 字段透传 | **不改代码**，仅核对没有硬编码旧默认值 |
| `gocloudagent/cli/shell_isolation.go:22-23` | 注释“为 nil 取默认（.git、.gocloudagent）” | 注释同步 |
| `plan/planv6.2/sandbox-research.md`、`gosandbox-refactor-plan.md` | 历史计划里描述旧默认 | **不改**（历史记录），但在本计划 §14 注明以本计划为准 |

### 9.2 依赖（允许为本计划引入第三方库）

| 库 | 用途 | 阶段 | 引入形态 | 说明 |
|---|---|---|---|---|
| ~~`github.com/bmatcuk/doublestar/v4`~~ | ~~glob 匹配与展开~~ | B | **未引入** | 实施时改为移植 srt `glob-pattern.ts` 自研（§3.2）：一份解析器同时产出 SBPL 正则、`DeniesRead` 匹配与 Linux 展开，避免两套引擎并存时的语义漂移，也不必改三处 `go.mod`。未对 `doublestar` 的具体行为做过实测，日后若要改用需先验证其对倒置范围等写法的处理 |
| ~~`pgregory.net/rapid`~~ | ~~属性测试~~ | B / P2-4 | **未引入** | 单一解析器后一致性测试不再需要；P2-4 如需随机化再评估 |
| `libseccomp-golang`（cgo） | Linux USER_NOTIF 违规观察 | — | **不引入** | 需要 cgo 与系统 libseccomp，违背 `gosandbox-inner`“纯 Go、静态”的现有约定（`gosandbox/README.md` 目录说明）；P2-6 才可能重评 |

**连带影响（D6 已决：同意）**：`gosandbox` 被根模块（`go.mod:17,106`）与 `gocloudagent`（`gocloudagent/go.mod:8,51`）以 `replace` 引用，`replace` 不传递。新增依赖后，**根 `go.mod`/`go.sum` 与 `gocloudagent/go.mod`/`go.sum` 需同步出现对应条目**。授权范围限定为：

- 仅允许 `go mod tidy` 产生的变更（新增 `doublestar` 及其传递依赖的 indirect 行与 `go.sum` 行）；
- 不得借机升级任何**已有**依赖版本；`go mod tidy` 若导致已有版本变动，须回退并单独说明；
- 不得改 `package.json` / `tsconfig.json` / `vite.config.ts`（本计划不涉及前端）；
- 验证命令：`cd gosandbox && go build ./... && go vet ./... && go test ./...`、`cd gocloudagent && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./...`、根 `go build ./internal/... ./cmd/...`；三处缺一不可（两侧 `go.mod` 各自声明 `replace`，只测一侧会漏）。
- 该授权原定随**阶段 B**（glob）生效；阶段 B 最终未引入依赖，三处 `go.mod` / `go.sum` 均未改动（`git status` 已核对）。

---

## 10. 关键测试矩阵

| 组 | 用例 | 平台 |
|---|---|---|
| 强制表 | 写 `.gitmodules` / `.vscode/*` / `.mcp.json` 失败；写普通文件成功 | Linux + macOS |
| 不存在路径 | 无 `.git` 时写 `.git/hooks/*` 失败（两平台一致）；`DenyRead` 指向不存在目录后沙盒内创建同名目录读到空掩蔽 | Linux + macOS |
| HOME 条件 | 可写根覆盖 HOME 时 `~/.bashrc` 不可写；不覆盖时不生成条目（argv 快照） | Linux + macOS |
| 不存在路径（D7） | 无 `.git` 时沙盒内 `git init` 成功且可 `commit`；`init` 后下一条命令写 `.git/hooks/x` 失败 | Linux + macOS |
| glob | `**/.env` 两层嵌套被拒；`[user-id]` 等拼不出集合的方括号按字面；`[WIP]` 按字符集；结尾分隔符被 `Validate` 拒绝；展开 >1024 报错；新建文件在 Linux 不受掩蔽（快照）、在 macOS 被拒、`DeniesRead` 命中 | Linux + macOS |
| 一致性 | ~~随机路径下 regex 与 doublestar 一致~~ 取消：三处共用同一份解析器（§3.2） | — |
| `.git` 粒度（D1） | 真 git：`init` / `add` / `commit` / `checkout` 成功；写 `.git/hooks`、`.git/config`、`.git/info/attributes` 失败；写 `.git/refs`、`objects` 成功；`.git` 为文件（worktree / submodule）时该文件只读，且 `gitdir:` 指向的目录内 hooks / config 受保护；`read_only_subpaths: [".git", ".gocloudagent"]` 显式配置时回到旧行为 | Linux + macOS |
| 违规 | macOS：触发一次越界写，`Result.Violations` 含带 token 的事件；噪声事件被过滤 | macOS |
| 违规（Linux） | `Violations` 为空且文档/信封如实说明“未采集” | Linux |
| 回归 | 现有 `run_linux_test.go` / `run_darwin_test.go` / `seatbelt_test.go` / `inner_linux_test.go` / `local_test.go` 全绿；网络全关用例逐字节不变 | 全部 |
| argv 快照 | 默认档 / 强制表档 / glob 档的 bwrap 参数黄金文件 | Linux |

---

## 11. 验收标准

1. 无 `.git` 的工作区里，沙盒内写 `.git/hooks/*` 在 Linux 与 macOS 均失败；`.gitmodules` / `.vscode` / `.idea` / `.mcp.json` 同样受保护。
2. `deny_read` 支持 `**/.env`、`**/*.pem`；Linux 快照语义与 macOS 前瞻语义均有测试并写入 README。
3. 无 `.git` 的工作区里沙盒内 `git init` + `commit` 可用；`init` 后的下一条命令写 `.git/hooks/*` 失败。
4. 沙盒内真 git 的 `init` / `add` / `commit` / `checkout` 可用，`.git/hooks`、`.git/config`、`.git/info/attributes` 与 `.git` 指针文件仍不可写；残余风险（`HEAD` / `refs` / `objects` 可写）已写入 `gosandbox/README.md`。
5. macOS 越界写能返回带归因的 `Violation`；Linux 如实报告未采集。
6. **网络行为不变**：全关档 argv / profile 与改动前一致（快照 + 现有集成测试双重证明）。
7. `gosandbox` 模块边界不变：无反向 import；新增依赖限于 §9.2 列出者；`internal/web/dist` 等无关目录零改动。
8. 文档：`gosandbox/README.md` 补强制表、“不存在路径不保护”的残余风险、glob 两侧语义、违规采集范围与 Linux 缺口。

---

## 12. 风险登记

| # | 风险 | 缓解 |
|---|---|---|
| R1 | ~~宿主临时挂载点残留~~ | **已消除**（D7：不引入占位挂载点） |
| R2 | “不存在就不保护”留下单条命令内 `git init` 后写 hook 的窗口 | 写入 README 残余风险；`init` 后下一条命令起即受保护 |
| R3 | glob 展开后挂载数量逼近 bwrap 参数上限（srt 9000 的教训） | 展开 1024 硬上限；超限报错；P1-3 `--args fd` 兜底 |
| R4 | macOS regex 与 Linux 展开两套语义漂移 | 已通过单一解析器消除匹配语义漂移；剩余的“快照 vs 前瞻”差异是有意的，已写入 README 与 `config.example.yaml`（D4） |
| R5 | `log stream` 在真机上的权限、延迟、噪声未验证 | 阶段 D 先做真机原型；失败则只保留通用 `Violation` 通道，不阻塞其他阶段 |
| R6 | macOS `sandbox-exec` 已标 deprecated，`with message` / regex 行为随系统版本漂移 | 只用稳定语法；`run_darwin_test.go` 加探针用例；长期替代方案另行跟踪 |
| R7 | 默认开启强制表是行为变更，可能影响既有 gocloudagent 用户脚本（如写 `.vscode`） | 先 warning 一个版本或提供 `ForceDenyWrite` 关闭路径；变更点写入 README |
| R8 | 新依赖连带根模块与 `gocloudagent` 的 `go.mod`/`go.sum` 变更（D6 已授权） | 阶段 B 未引入依赖，此风险未触发；后续如引入，授权限于 `go mod tidy` 产生的新增行，不升级既有依赖（§9.2），三处构建缺一不可 |
| R9 | 祖先 pin 可能是为不存在的洞做工 | §4.2 先复现再做 |
| R10 | D1 放开 `.git` 后，可写的 `HEAD` / `refs` / `objects` 允许改写分支指向、植入对象（不直接导致宿主执行，但影响用户之后检出/合并的内容） | 写入 README 残余风险；hooks / config / attributes / 指针文件保持强制保护；操作者可显式配回 `.git` 整体只读 |
| R11 | D1 的“默认行为变更”若与强制表不同批发布，会出现 `.git` 已放开而 hooks 未单独保护的安全回退 | 阶段 C 硬性排在阶段 A 之后并同批发布；§9.1 联动清单作为评审检查表 |

---

## 13. 工作量估算

| 阶段 | 内容 | 估算 |
|---|---|---|
| 0 | 契约冻结（D1、D6 已决） | 0.5d |
| A | P0-1 写侧强制 deny（D7 后不含挂载点生命周期模块） | 1.5–2.5d |
| B | P0-2 glob（已完成；无需 `go.mod` 同步，实际约 2d） | ~~3–3.5d~~ 2d |
| C | P0-3 `.git` 粒度（含联动清单 9 处同步） | 1–1.5d |
| D | P1-1 违规监控 | 3–4d |
| E | P1-2 / P1-3 | 1–3d（有条件） |
| 合计 | P0+P1 | **10.5–15d**（扣除阶段 B 的节省） |
| F | P2 | 按需，逐项 0.5–3d |

单人净工时；阶段 D 的 macOS 真机原型若失败，D 段缩到 1–1.5d（仅通用通道）。阶段 A、C 必须同批发布（R11）。

---

## 14. 附录：证据索引

### srt（相对 `thirdparty/sandbox-runtime/`）

- 配置模型：`src/sandbox/sandbox-schemas.ts`（:1-48 读 deny-then-allow、:66-71 写 allow-only）
- 强制 deny 表：`src/sandbox/sandbox-utils.ts`（:21-46 `DANGEROUS_FILES` / `DANGEROUS_DIRECTORIES`）、`README.md`（:1142-1190）
- Linux：`src/sandbox/linux-sandbox-utils.ts`（:661-665/1141-1149 挂载点管理、:862-863 参数上限、:1028-1067 O_TMPFILE、:1886-1912 祖先 pin、:3603-3609 归因编码、:3851-3869 userns）
- seccomp：`vendor/seccomp-src/apply-seccomp.c`（:11-30 两段式布局、:32-39 userns 双保险、:781-788 DUMPABLE）
- macOS：`src/sandbox/macos-sandbox-utils.ts`（:164-216 glob→regex、:932-961 搬移阻断、:1081-1083 `with message`、:1627-1659 log monitor）
- 违规：`src/sandbox/linux-violation-monitor.ts`（:20-105）
- glob：`src/sandbox/glob-pattern.ts`（:8-23 token、:48-76 方括号容错）
- 文档：`README.md`（:117-120 读写优先级、:560-584 glob 边界、:591/1278 弱化降级、:1128 残留挂载点、:1140 错误码、:1168-1176 不存在路径与 `git init`）

### gosandbox（相对 `gosandbox/`）

- 策略：`host/policy.go`（:26-30 三档、:47 默认只读子路径、:50-75 字段、:91-96 nil 语义、:116-120 校验、:135-159 cpu_max、:161-175 DeniesRead）
- Linux 执行：`host/bwrap_args.go`（:37-39 顺序、:68-128 参数全集、:93 只读子路径跳过不存在、:99-110 deny_read、:114-116 --disable-userns、:118-119 policy fd）、`host/run_linux.go`、`host/start_linux.go`
- inner：`host/inner/inner_linux.go`（:5 顺序、:55-71 Landlock、:81-99 seccomp、:110-115 ABI）
- macOS：`host/seatbelt.go`（:14-29 模板、:23 全局读、:60-73 deny 后置）、`host/run_darwin.go`
- cgroup：`host/cgroup_linux.go`、`host/probe_linux.go`
- vm：`vm/driver.go`、`vm/local/local.go`（:216-273 ProtectDirs、:276-284 baseEnv）
- 调用方：`gocloudagent/cli/shell_isolation.go`（:21-24 writable_roots / read_only_subpaths / deny_read）、`gocloudagent/tools/tools.go`（:218-222 `limits=unenforced` 信封）
- `.git` 默认值联动点（D1，见 §9.1）：`host/policy_test.go`（:123-141,181-207）、`host/seatbelt_test.go`（:67,83）、`host/run_linux_test.go`（:97-102）、`host/run_darwin_test.go`（:73-78）、`README.md`（:29,56）、`docs/automation/sandbox.md`（:44）、`config.example.yaml`（:212）、`internal/config/config.go`（:252）、`internal/config/yaml.go`（:198）
- 模块引用：根 `go.mod`（:17,106）、`gocloudagent/go.mod`（:8,51）对 `gosandbox` 的 `replace`（D6）
- 历史计划（以本计划为准，不回改）：`plan/planv6.2/sandbox-research.md`（:362,615,759 旧默认与“默认保留 `.git` 只读”的结论）、`plan/planv6.2/gosandbox-refactor-plan.md`（:155,170）
- 边界约束：`internal/AGENTS.md`（:35-37）、根 `AGENTS.md` 硬性约束 4
- 文档：`README.md`（:49 cgroup 未实测、:53-57 已知限制、:75-82 性能）

---

## 15. 执行记录

### 已完成：阶段 0、A、C（2026-10-26）

| 项 | 实现 | 位置 |
|---|---|---|
| 强制 deny 表 | `ForceDenyWriteSubpaths`（`.git/hooks`、`.git/config`、`.git/info/attributes`、`.gitmodules`、`.mcp.json`、`.vscode`、`.idea`）；HOME 下 shell/git 配置仅在可写根覆盖 HOME 时追加；只保护已存在路径（D7） | `gosandbox/host/mandatory.go` |
| 开关 | `Policy.DisableForceDeny`（反向命名，零值即开启）；关闭时 `Result.Warnings` 如实提示 | `host/policy.go`、`host/run.go` |
| 两平台接入 | Linux `--ro-bind`（在可写 bind 之后）；macOS `deny file-write*` 由“不判存在”改为“仅已存在路径”，与 Linux 一致 | `host/bwrap_args.go`、`host/seatbelt.go` |
| `.git` 粒度（D1） | `DefaultReadOnlySubpaths` 由 `[.git, .gocloudagent]` 改为 `[.gocloudagent]` | `host/policy.go` |
| 信封透出 | shell 工具返回里新增 `warnings`（无提示时不出现） | `gocloudagent/tools/tools.go` |
| 联动同步 | README、`docs/automation/sandbox.md`、`config.example.yaml`、`shell_isolation.go` 注释、4 个旧测试 | 见 §9.1 清单 |
| 新增测试 | `mandatory_test.go`（纯逻辑，跨平台）；macOS 真 sandbox-exec 7 条（含真 git init/commit、init 后 hooks 受保护）；Linux 对应 6 条；`vm/local` 真隔离 1 条 | `host/*_test.go`、`vm/local/local_test.go` |

**偏离原计划的地方（均有依据）**：

1. 阶段 A 原含“Linux 不存在路径占位 + 临时挂载点生命周期模块”，**整体删除**（D7）。依据 macOS 真机实测：对不存在的 `.git/hooks` 加 deny，沙盒内 `git init` 直接失败。
2. `ForceDenyWrite`（默认开）实际命名为 `DisableForceDeny`，避免 Go 零值把“忘记设置”变成“关闭保护”。
3. `.git` 内保护范围比 §3.3 表格少一项：**未实现“`.git` 为文件（worktree/submodule）时跟随 `gitdir:` 指针”**。原因：该场景需要读取并解析 `.git` 文件，且指向目录常在工作区之外，价值与复杂度需单独评估，见下方未完成项。

### 验证状态（如实）

| 验证 | 结果 |
|---|---|
| `gosandbox`：`go build` / `go vet` / `go test ./...` | 通过（macOS） |
| `GOOS=linux go vet ./host/... ./cmd/...` | 通过（仅静态检查） |
| `gocloudagent`：`GOWORK=off go build/vet/test ./...` | 通过 |
| 根：`go build ./internal/... ./cmd/...`、`internal/{config,sandbox,agentbridge}` 测试 | 通过 |
| macOS 真 `sandbox-exec` 用例（含真 git、`TestDarwinDenyReadGlobRecursive`） | 通过 |
| `internal/config`、`gocloudagent/{cli,tools}` 测试；根 `go build ./internal/... ./cmd/...` | 通过（本轮重跑） |
| **Linux 真 bwrap 用例** | **未运行**：开发机是 macOS，无 `bwrap`。`run_linux_test.go` 新增的 7 条用例（含本轮的 `TestLinuxDenyReadGlobEnv`，其“新建文件不受掩蔽”的快照断言只在 Linux 真跑过才算验证）与 `vm/local` 的 git 用例在 Linux 上会真跑，**需在有 bwrap 的 Linux 机器上补测**，其中 `TestLinuxGitInitAndCommit` 与 `TestLinuxGitHooksProtectedAfterInit` 最关键（验证 bwrap 下 `git init` 不受强制表影响） |
| 根模块 `go test ./...` 全量 | 未跑（只跑了受影响的包） |

### 已完成：阶段 B glob（2026-10-26）

| 项 | 实现 | 位置 |
|---|---|---|
| 解析器 | 移植 srt `glob-pattern.ts`：`*` / `**` / `?` / `[集合]`，无转义；拼不出集合的方括号（空、范围倒置如 `[user-id]`、无 `]`）按字面；连续通配符合并 | `host/glob.go` |
| 校验 | `Validate` 要求绝对路径；glob 不得以 `/` 结尾且正则可编译；`literal:` 前缀强制字面量 | `host/glob.go` `validateDenyEntry` |
| Linux | `ExpandGlob` 展开为当下存在的路径（快照语义），单 glob 上限 `MaxGlobMatches=1024`，另有遍历上限 200000 项，超限返回 `ErrGlobTooLarge`，不截断 | `host/glob.go`、`host/bwrap_args.go` |
| macOS | glob 转 SBPL 正则，覆盖未来新建文件 | `host/seatbelt.go` |
| 父进程文件工具 | `DeniesRead` 按完整 glob 判断 | `host/policy.go` |
| 配置层 | 沿用 `internal/config` 的“绝对路径或以 `~` 开头”校验，`gocloudagent` 的 `expandHome` 只处理开头的 `~/`，不破坏 glob 条目 | 未改代码，已核对 |

**偏离原计划**：不引入 `doublestar`，三处 `go.mod` / `go.sum` 零改动（`grep doublestar` 为空）。取消一致性属性测试。理由与方括号语义更正见 §3.2、§9.2。

**本轮补充**：

- 两平台各补一条端到端用例，直接验证 §3.2 的两侧语义：`TestDarwinDenyReadGlobRecursive`（`ws/**/*.pem`：已有的根层与深层文件读不到，沙盒内新建的 `.pem` 同样读不到）；`TestLinuxDenyReadGlobEnv`（`ws/**/.env`：已有的根层与深层 `.env` 被掩蔽，沙盒内新建的匹配文件**读得到**，即快照语义）。
- `config.example.yaml` 的 `deny_read` 补 glob 写法、绝对路径限制（`**/.env` 这种相对写法会被配置校验拒绝，示例用 `~/projects/**/.env`）与两平台语义差异。

### 未完成

- 阶段 D：macOS 违规监控。
- 阶段 E：祖先 pin 复现、`--args fd`。
- 阶段 C 遗漏项：`.git` 为文件（worktree/submodule）时的 `gitdir:` 跟随保护；需先评估再决定做不做。
- 阶段 A 遗漏项：argv 快照（黄金文件）断言已有顺序断言兜底，未单独做黄金文件。
