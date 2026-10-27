# 后端优化与精简 v3 实施计划（复核修订版）

> 日期：2026-10-10（12:21 复核修订）
> 范围：`internal/`、`gofunction/`，以及 `cmd/` 编译与测试。
> 状态：**待实施**。本次只更新计划，没有修改业务代码。
> 目标：先修复已复现或源码可确定的权限、竞态和生命周期缺陷；再证明代码确实无用后删除、合并重复逻辑；同时补充能拦住这些缺陷的测试。行数和覆盖率不作为完成标准。

## 0. 复核结论：上一版遗漏与误判

上一版只抽查了少量文件，**没有跑后端全量测试，也没有跑 race**，因此有三处判断不准确：

1. **遗漏了已经能复现的数据竞争**。`go test -race` 在 `internal/api` 的 `TestAgentSkillsE2E` 中报告 5 处 DATA RACE，位置是 `agent_handler.go:744` 的 SSE 事件序号 `seq++`。上一版误以为这个竞态“当前已有锁”。
2. **遗漏了权限问题**：API Key 吊销没有限定项目；低权限 Key 可以签发高权限 Key；Agent 委托令牌可以访问用户和 API Key 管理接口（见 §2.1）。
3. **优先级排错**。`gofunction.ExecutorPool` 在生产代码中**没有调用方**，生产只走 `gofunction.RunJSON`。池的计数缺陷是真的，但不是线上主路径；`RunJSON` 不响应调用方取消、输出缓冲存在并发写，才是生产问题（见 §2.3）。

另外更正：只读实例（`instance.writable=false`）在 `app.go:420` 处直接返回，**不会启动系统库**，依赖系统库的路由都不会挂载。所以 agent/settings/users 等 handler 缺少 `writable` 检查，目前只是纵深防御缺口，不是只读实例上已经可写的漏洞，优先级下调（见 §2.4）。

## 1. 约束与证据口径

- 遵守根目录和 `internal/AGENTS.md`；解释器以 `gofunction/README.md` 为契约，文档与源码不一致时先核实，再更新文档。
- 工作区已有大量未提交修改。实施前先记录差异，不覆盖、不回退、不自动提交；**行为修复、精简删除、纯文件搬移分开交付**。
- 保持路由、错误码、JSON/SSE 字段、权限和审计协议不变。如果修复必须改变外部行为（例如吊销其他项目的 Key 从 204 改为 404），在对应条目中明确写出。
- 保持单写实例、`committed_local`、异步发布 catalog 的语义。不新增依赖、不改 `go.mod` 或配置结构、不操作生产 S3、不执行数据保留清理。
- 证据分三级：
  - **已复现**：有命令或失败用例；
  - **静态确认**：源码路径唯一，可以直接写出失败用例；
  - **风险候选**：必须先复现，才允许修改行为。

## 2. 缺陷台账

### 2.1 权限与鉴权（P0）

| 编号 | 等级 | 证据 | 影响 | 修复方向与必须新增的测试 |
| --- | --- | --- | --- | --- |
| SEC-01 | 静态确认 | `auth/api_key_repository.go:76-77` 执行 `UPDATE sys_api_keys SET revoked_at=? WHERE id=?`，没有 `project_id` 条件；`api_keys_handler.go:156-180` 只检查当前项目的访问权，而不检查 Key 的归属项目 | 项目 A 有权限的主体只要拿到项目 B 的 Key ID，就能吊销它。另外，不存在的 ID 也返回 204 | 仓储增加按 `(id, project_id)` 吊销并检查受影响行数；不存在或不属于本项目时统一返回 404；重复吊销的语义要单独确定。测试：跨项目吊销后对方 Key 仍能认证、不存在的 ID、重复吊销 |
| SEC-02 | 静态确认 | `api_keys_handler.go:184-216`：API Key 通道只要持有 `ProjectAdmin` 就能签发；签发的权限不受签发者自身权限限制，权限列表为空时默认授予全部权限 | 只有 `ProjectAdmin` 的 Key 可以签发带 `DatabaseWrite/DatabaseAdmin` 的新 Key，造成权限提升 | 新 Key 的权限必须是签发者权限的子集；登录态按角色展开后的权限求子集；空列表改为“签发者已有的权限”。这一改动会影响现有调用方，需要在发布说明中写明。测试：子集签发成功、越权签发返回 403、空列表的结果 |
| SEC-03 | 静态确认，需端到端复现 | `auth/middleware.go:96-102`：委托令牌只拒绝 `/v1/auth*` 和 `/llm/providers`，其余路由都放行（黑名单）；`PrincipalFromDelegation` 会还原 `UserID/Role`。因此委托主体可以通过 `canIssue` 签发**永久** API Key，superadmin 发起的委托还能调用 `/v1/users` 写接口 | 有效期 30 分钟的 Agent 委托令牌可以换成长期凭据，绕过 run 结束时的撤销 | 改为白名单：委托主体只能访问 Agent CLI 需要的项目内业务路由；`/api-keys`、`/users`、`/projects` 的创建、settings 和凭据接口一律拒绝。先用 httptest 证明当前可以签发成功，再修复。测试：委托令牌访问上述接口全部 403，正常 CLI 命令不受影响 |
| SEC-04 | 风险候选 | `CancelRun`、`ConfirmRun` 只要求 `DatabaseRead`，并且只校验 run 属于本项目，不校验发起人 | 同一项目的只读主体可以取消或确认他人的运行，包括确认破坏性 CLI 操作 | 先由产品确认项目内是否允许互相操作；如果不允许，记录 run 的发起主体并校验。破坏性确认至少需要写权限 |

### 2.2 并发与生命周期（P0/P1）

| 编号 | 等级 | 证据 | 影响 | 修复方向与必须新增的测试 |
| --- | --- | --- | --- | --- |
| RACE-01 | **已复现** | `go test -race -short ./internal/api` 失败：`TestAgentSkillsE2E` 的 `6_system_database/7_function/8_cron/confirmation_timeout` 共 5 处 DATA RACE。`agent_handler.go:739-751` 中，`seq++`、`events.append` 与 `writeBytes` 不在同一把锁里，StartRun 回调和 thinking/心跳 goroutine 会并发调用 `write` | 事件 ID 可能重复或乱序（例如先发出 id 5 再发 id 4），断线续传时客户端可能漏收或重复收到事件 | 在同一把锁内完成分配 ID、写入事件缓存和输出；heartbeat 与 thinking 统一走这个写入函数，并在函数返回前保证 goroutine 已退出。测试：并发写 N 次后 ID 严格递增且不重复；race 下重复运行 |
| LIFE-01 | 静态确认 | `cronjob_handler.go:403` 和 `agent_schedule_handler.go:332-335` 的手动触发使用 `context.WithoutCancel(request ctx)`，不受 `schedulerBaseCtx` 控制 | `App.Shutdown` 取消调度器后，手动触发的任务不会被取消。`Stop()` 一直等待，直到 shutdown 超时 | 手动触发改用调度器自己的 ctx（由 Start 保存），与请求 ctx 解耦，但受停止信号控制。测试：手动任务阻塞时调用 Stop，任务被取消且 Stop 有界返回 |
| LIFE-02 | 静态确认 | 两个调度器在 `Trigger/tickOnce` 中调用 `wg.Add`，而 `Stop` 只做 `wg.Wait`，没有“已停止”状态 | Stop 期间或之后仍可能调用 `Add`，违反 WaitGroup 约束，可能 panic，或者让 Stop 漏等任务 | 增加 `stopping` 状态，停止后拒绝 Trigger 并返回明确错误；Add 与停止状态在同一把锁内判断。测试：Stop 与 Trigger 并发、重复 Stop |
| LIFE-03 | 静态确认 | `agent_schedule_handler.go:333-336` 在 goroutine 中调用 `Trigger` 并丢弃错误；cronjob 的 Trigger 会把“已在执行”映射为 409 | 定时 Agent 已在执行或调度器已停止时，接口仍然返回 202，与 cron 接口不一致 | 同步调用 Trigger（它本身就是异步的）并映射错误；`Trigger` 文档写着返回记录 ID，实际总是返回 `""`，需要修正文档或补齐实现。测试：重复触发返回 409 |
| LIFE-04 | 风险候选 | `CancelRun` 先读取状态再写 `canceled`，`finishAgentRun` 同时可能写 `completed` | 最终状态取决于谁后写：已完成的运行可能被改成取消，或者反过来 | 状态只允许单向迁移，使用条件更新（`WHERE status IN (running, queued)`）；先在测试中用 barrier 复现交错 |
| LIFE-05 | 风险候选 | `run_events.go` 只有收到 `end` 事件才安排清理 | 中途 panic 或没有写出 end 的运行，事件缓存会一直留在内存 | 在 `streamRun` 中用 defer 保证结束时标记完成；增加缓存总量上限 |

### 2.3 云函数解释器（生产路径 `RunJSON`，P1）

| 编号 | 等级 | 证据 | 影响 | 修复方向与必须新增的测试 |
| --- | --- | --- | --- | --- |
| GF-RUN-01 | 静态确认 | `jsonrun.go:116-176` 只在执行前后检查调用方 ctx；`Program.Run` 内部使用 `newCallContext()`，固定 10s 超时，与调用方 ctx 无关 | 客户端断开、请求超时或服务关闭时，脚本仍然继续执行 | 增加接收父 ctx 的内部执行入口，取消时在块边界停止；公开 API 保持兼容。测试：死循环脚本在父 ctx 取消后有界返回，错误类型可以区分取消与超时 |
| GF-RUN-02 | 静态确认 | `program.go:158` 在返回前额外调用 `waitGoroutines(10s)`；等待超时后，脚本中的 goroutine 仍在后台运行 | 单次调用最长约 20s；泄漏的 goroutine 可能继续调用宿主 HTTP 等函数 | 等待时间纳入同一个截止时间；ctx 取消后，脚本 goroutine 在块边界退出；记录并测试“无法强制终止宿主阻塞调用”这一边界 |
| GF-RUN-03 | 静态确认 | `frame.go:35` 的 `outBuffer` 是 `strings.Builder`；`call.go:245` 的 print 与 `eval.go:607` 的 goroutine panic 可能从多个 goroutine 写入 | 脚本 goroutine 调用 print 时出现数据竞争，输出可能损坏 | 给输出缓冲加锁并设置大小上限。测试：多个 goroutine 同时 print，在 race 下运行 |
| GF-RUN-04 | 风险候选 | `/go` 调用和 TestVersion 没有并发上限；每次调用都重新编译 | 恶意或高频调用可能耗尽 CPU 和内存 | 先压测；如需修复，按项目或全局设置信号量，满载时返回 429/503，并补配置说明 |

### 2.4 写门禁与系统库同步（P1/P2）

| 编号 | 等级 | 证据 | 影响 | 修复方向与必须新增的测试 |
| --- | --- | --- | --- | --- |
| WR-01 | 静态确认（纵深防御） | `CreateRun`/`CancelRun`/`ConfirmRun`、users、auth 改密、api-keys、projects 创建、settings/retention/llm settings/provider 凭据等写接口都没有 `writable` 检查 | 当前只读实例不启动系统库，这些路由不会挂载，因此不能直接利用；但以后如果引入“只读实例也打开系统库”，会立即变成可写漏洞 | 在路由层统一提供写门禁中间件，或给 handler 统一加检查；写接口清单用路由表驱动测试，防止新增路由漏挂 |
| SYNC-01 | 风险候选（安全相关） | `auth` 的 user/session/api key 仓储直接对 `Store.DB()` 执行 `ExecContext`，**不调用 `notifyWrite/MarkDirty`**；`catalog_syncer.go:435` 在已有 manifest 且 dirty=0 时跳过同步 | 吊销 Key、改密或禁用用户如果只有这一次写入，可能要等其他写入顺带标脏才会发布；如果在此之前崩溃，恢复后的 catalog 中 Key 仍然有效 | 先在内存 BlobStore 中复现“只发生 auth 写 → Close → 冷恢复”；确认后给 auth 仓储注入写后回调，或在 systemdb 层统一包装 |

### 2.5 低优先级与已更正项

- **执行器池（`ExecutorPool`，非生产路径）**：`executor.go` 中重复 Close 会重复入池、池关闭后计数可能为负、`reset` 不清除中断标记、旧定时器可能误取消新执行，这四个问题都是静态确认。生产代码没有调用方，所以排在 RunJSON 之后修复，但仍然要补失败测试，因为它是公开 API。
- **KV `EXISTS`** 已改为调用 `KeyRepo.Exists`（`kv_commands.go:98-106`）。只保留 TTL、重复参数、空 schema 的回归测试，不再列为缺陷。
- **工具确认等待**：`cli_tool.go` 已处理 ctx 取消；确认超时视为拒绝，这一点在测试中保留。

## 3. 本轮实测基线（2026-10-10）

| 命令 | 结果 |
| --- | --- |
| `go vet ./internal/... ./gofunction/... ./cmd/...` | 通过，无输出 |
| `go test -short -count=1 -parallel 1 -timeout 20m ./internal/... ./cmd/...` | 全部通过；`database/ducklake` 约 142s、`lease` 31s、`app`/`api` 约 21s |
| `go test -short -count=1 ./gofunction/...` | 通过（主包约 11s） |
| `go test -race -short -count=1 -parallel 1 ./internal/api ./internal/auth ./internal/gocloudagent ./internal/cronjob ./internal/database/registry ./internal/sandbox ./internal/systemdb ./gofunction/...` | **`internal/api` 失败**：RACE-01，共 5 处；其余包通过 |
| `gofmt -l internal gofunction cmd` | 32 个文件未格式化（`gofunction` 与 `cmd` 为 0） |

尚未运行：全部包的 race、覆盖率、benchmark、非 short 的外网用例、真实 S3。所以第 2 节中“静态确认”的条目仍需在实施时先写出失败用例。

## 4. 实施顺序与验收门

| 批次 | 内容 | 门槛 |
| --- | --- | --- |
| A | 补齐基线：剩余包的 race、分包覆盖率、易抖动测试清单 | 只记录，不改行为 |
| B1 | SEC-01/02/03：先写失败用例，再修复 | 越权用例全部变为 403/404；正常签发和 CLI 命令不受影响；外部行为变化写入说明 |
| B2 | RACE-01，以及 LIFE-01/02/03 | `go test -race ./internal/api ./internal/cronjob ./internal/gocloudagent` 连续 `-count=5` 无 race；Stop 有界返回 |
| B3 | GF-RUN-01/02/03 | 取消后有界返回；print 并发在 race 下通过；`RunJSON` 签名不变 |
| C | 先复现 SEC-04、LIFE-04/05、SYNC-01、GF-RUN-04，确认后再修 | 复现不了的条目保持“候选”，不修改行为 |
| D | WR-01 写门禁统一、执行器池修复 | 路由表驱动测试覆盖全部写接口 |
| E | 精简与去重（第 5 节） | 前面各批的回归全部通过 |
| F | 文档与性能复核 | README 中的取消语义与实现一致 |

每项修复都按同一流程：先写出失败的回归测试 → 最小修复 → 运行该包的 race → 单独交付。

## 5. 精简与去重

### 5.1 删除准入条件

候选必须在整个仓库中确认没有用途：包括 `cmd/`、测试、脚本、接口实现、反射/SSA 名字查找、`init` 注册和导出 API。文本搜索没有引用，不等于可以删除。

| 候选 | 结论 | 处理 |
| --- | --- | --- |
| `Executor.functionChecksums` | 只在初始化和 reset 时赋值，没有读取的地方 | 可以删除私有字段及其分配 |
| `DelegationRegistry.Remember` 的 `exp` 参数（`_ = exp`） | 没有使用；作废时固定按 31 分钟计算 | 改为用 exp 计算作废记录的保留时间，或者删除参数（属于内部 API） |
| `Executor.Compile/SetProfile/GetProfile/SetWaitEventLoop`、`Runtime.Cleanup` | 公开的预留接口 | 不删；在 README 中标为预留。以后如果要弃用，走单独的兼容流程 |
| `ExecutorPool` 整体 | 生产代码没有调用，但属于公开 API 并有测试样例 | 保留并修复；是否弃用另行决定 |
| `coverage_*`/`gaps`/`cheap` 测试文件 | **已审查（2026-10-10）**：17 个文件 83 个用例、448 处断言，平均每用例 5+ 断言，绝大多数断言落在具体返回值/错误分类/panic 消息上 | 两个弱用例已加强：`gofunction/cheap_test.go` `TestExecutorSetProfileAndCleanup` 补预留接口契约断言（GetProfile 恒 nil、Cleanup 不破坏状态）；`observability/zaplog_coverage_test.go` `TestFieldAliases` 补别名绑定一致性断言（Key+Type 与 zap 原生对拍）。其余保留 |

### 5.2 去重与拆分（纯搬移单独交付）

| 文件 | 行数 | 方式 | 不变项 |
| --- | --- | --- | --- |
| `api/kv_commands.go` | 1422 | 按数据类型拆分文件，用一个函数显式组装命令表，并检测重复注册 | 命令行为、TTL、empty 分支 |
| `api/agent_handler.go` | 941 | 在 RACE-01 修复之后，把 SSE 写入抽成独立结构 | 事件协议、收尾顺序 |
| `app/app.go` | 1083 | 按装配阶段拆分函数，保留统一的回收栈 | 唯一装配点、逆序 Close |
| `sbcli/commands.go` | 1223 | 按领域拆分 | 命令名、退出码、输出 |
| `ducklake/catalog_syncer.go` | 1172 | 按捕获、发布、水位拆分 | 写锁、manifest 提交点 |
| `sandbox/manager.go` | 934 | 按生命周期、路径、配额拆分 | Driver 边界 |
| 两个调度器 | — | ~~可以抽出共用的生命周期实现~~ **已评估（2026-10-10）：不抽取**。Trigger 签名不同（cronjob 返回 `error`，gocloudagent 返回 `(string, error)` 且有独有 `ErrScheduleExecuting`），统一需改公开 API；抽取必须注入闭包转发 tickOnce/execute，违反"不增加只做转发的薄封装层"原则；且 stopped 判定与 wg.Add 同锁的不变式（LIFE-02）分散到共享类型后出错面更大 | 各自的执行与落库逻辑 |
| `NewHTTPError(status, "裸字符串")` | 75 处 | 按模块逐步替换为领域错误 | HTTP 状态码与 `error.code` |

只有能降低耦合时才拆分；不新建通用框架，也不增加只做转发的薄封装层。

## 6. 测试补强清单

1. **权限**：跨项目吊销、越权签发、委托令牌访问管理接口、只读角色访问全部写路由（路由表驱动）。
2. **并发**：SSE 事件 ID 单调；调度器 Stop 与 Trigger 交错；Cancel 与完成交错；print 并发。全部在 `-race -count=5` 下运行。
3. **生命周期**：每个 `app` 依赖初始化失败时只回收已经创建的资源；Shutdown 超时后不残留 goroutine（可用 goroutine 计数做有界检查）。
4. **同步一致性**：只有 auth 写入时的冷恢复（SYNC-01）；flush、PUT 或 manifest 失败时水位不前进。
5. **解释器**：与原生 Go 对拍（typed nil、接口断言、闭包捕获、defer/recover、整数边界、select）；JSON 绑定边界；fuzz 只覆盖纯函数（参数解析、cursor、路径）。
6. 断言必须落在状态码、错误类型、事件序列或资源计数上。禁止只断言“不 panic”，也禁止为了覆盖率调用空实现。

## 7. 验收命令

```sh
# 每批（把 PKGS 换成实际改动的包）
go test -short -count=1 $PKGS
go test -race -short -count=5 -parallel 1 $PKGS
go vet $PKGS
gofmt -l $(git diff --name-only -- '*.go')
git diff --check

# 收尾
go build ./internal/... ./cmd/... ./gofunction/...
go test -short -count=1 -parallel 1 -timeout 20m ./...
go test -race -short -count=1 -parallel 1 -timeout 30m ./internal/... ./gofunction/...
go vet ./internal/... ./gofunction/... ./cmd/...
```

- 收尾时 race 必须全部通过：RACE-01 修复前，`internal/api` 的失败就是已知的阻塞项。
- 超时要定位到具体用例，不能靠加大 `-timeout` 掩盖死锁。
- 没有外网或真实 S3 授权时，在报告中写明“未执行”。

## 8. 完成定义

- §2 的 P0 条目（SEC-01/02/03、RACE-01）都有“修复前失败、修复后通过”的回归测试。
- 后端全量 race 通过；两个调度器的 Stop 都能有界返回，并能取消手动触发的任务。
- `RunJSON` 能响应调用方取消；README 的取消与超时说明与实现一致，并写明宿主阻塞调用无法强制终止。
- 删除的每项代码都有用途审查记录；公开接口、迁移与安全边界没有被误删。
- 风险候选条目要么已复现并修复，要么保留为候选并写明原因。
