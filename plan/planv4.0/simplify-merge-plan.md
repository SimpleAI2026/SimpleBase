# SimpleBase v4.0 UI + 后端精简合并计划

> 日期：2026-10-06
> 范围：`internal/`、`ui/src/`、`ui/tests/`、仓库根遗留产物
> 前置文档：同目录 `cleanup-plan.md`（后端，v1.1）、`ui-cleanup-plan.md`（前端，v1.2 已执行）
> 方法：非测试代码引用追踪（grep）+ UI/JS-SDK/Go-SDK/docs 端点消费核对 + `Api` 契约方法逐项消费扫描 + 重复实现比对
> 基线：`go build ./internal/... ./cmd/...` 通过（2026-10-06 复核）
> 原则：只删「零消费」与合并「重复实现」，不改对外协议字段、不新增依赖、不动 `go.mod` / `package.json` / `tsconfig.json` / `vite.config.ts`

---

## 0. 结论总览

| 组 | 含义 | 数量 | 风险 |
| --- | --- | --- | --- |
| R | 旧计划遗留未执行项（复核仍存在） | 7 | 零/低 |
| S | 后端死代码删除 | 6 | 低 |
| M | 后端重复代码合并 | 5 | 低 |
| U | 前端死代码删除 | 4 | 低 |
| V | 前端重复代码合并 | 5 | 低 |
| T | 测试文件归并 | 2 | 低 |
| D | 需决策项（默认本轮不动） | 5 | 中 |
| K | 防误删清单 | — | — |

预计收益：后端删除约 900 行非测试代码（含测试约 1500 行）、前端删除约 400 行业务代码 + 1 个组件、合并 3 类重复工具函数（约 100 处调用点收敛）、git 仓库瘦身 17MB+。

**前置条件**：工作区当前有 59 个未跟踪 + 115 个修改 + 81 个删除（sandbox 功能、docs、examples 重组）。**必须先把现有功能改动提交**，再按本计划逐项独立 commit，避免清理与功能混杂。

---

## R. 旧计划遗留项（`cleanup-plan.md` 中仍未执行，2026-10-06 复核仍存在）

| 编号 | 原编号 | 内容 | 复核证据 | 动作 |
| --- | --- | --- | --- | --- |
| R1 | A1 | `gofunction/gofunction.test`（17MB Mach-O）仍被 git 跟踪 | `git ls-files` 命中 | `git rm --cached` + `.gitignore` 补 `*.test` + 删本地 |
| R2 | A2/A3/A4 | `datasets/`、`output/cfbee745-*`、`skills/`（25 文件）仍被跟踪 | `git ls-files` 命中 | `git rm -r` |
| R3 | A6 | 根目录 `simplebased`（135MB）本地产物 | 已 gitignore | 本地删除 |
| R4 | A9 | `.github/workflows/build-image.yml` 仍 `cd cmd/http`、Go 1.20、lessdb 镜像名 | 文件内容未变 | 对齐 Go 1.25 + `go build ./...` + 现有 Dockerfile，或删除 |
| R5 | B1 | `internal/systemdb/gofunctions.go` 兼容层（6 个方法）仍在 | 非测试调用 0 处（仅 `gofunctions_test.go`） | 删除该文件及其测试；`cron_jobs.go` 注释改 `sys_go_funcs.name`；`sys_gofunctions` DDL 保留 |
| R6 | C1 | 根 `README.md` 仍链接 `plan/plan.md`、仍列 `litellm/` 目录 | grep 命中 137/160 行 | 修链接，按实际目录重写结构节 |
| R7 | C3 | `internal/database/README.md` 仍有 `turso/` 小节 | grep 命中 10-11 行 | 删 turso 小节与幽灵文件条目，补 `kv/`、`lease/` |

已确认执行完毕（无需再做）：B2 legacy 路由、B3 open/close 桩、B4 `LastInsertID`、B5 KVInitializer 双构造、B6 `sys_log_exports`。

---

## S. 后端死代码删除

### S1. `internal/sandbox/client.go` 旧版 Client 兼容层（约 340 行 + `client_test.go`）
- **证据**：`sandbox.New`、`*Client`、`ThreadSandbox`、`normalizeConfig`、`validPath`、`truncateBytes`、`wrapExecErr`、`SandboxName`、`threadLock`、`networkPolicy` 在 client.go 之外的非测试代码**引用 0 处**；生产链路已统一走 `Manager` + `Driver`（`app.go:887 sandboxAdapter{m *sandbox.Manager}`）。`driver_cloud.go:15` 注释自称「除本文件与兼容层 client.go 外不使用 SDK」。
- **动作**：
  1. 删除 `client.go`、`client_test.go`；
  2. 将仍有价值的测试（路径防穿越、截断）确认已被 `manager`/`paths.go` 测试覆盖，否则迁移到 `paths_test.go`；
  3. 修订 `cloudagent/access.go:99` 注释「实现方为 internal/sandbox.Client」→「实现方为 app.sandboxAdapter（包装 sandbox.Manager）」；修订 `driver_cloud.go:15` 注释。
- **验证**：`go build ./... && go test ./internal/sandbox/... ./internal/app/... ./internal/cloudagent/...`；`grep -rn "microsandbox" internal --include='*.go' -l` 只剩 `driver_cloud*.go`。

### S2. `GoFunctionHandler.Update`（旧 PUT 兼容）
- **证据**：`gofunction_handler.go:364-368` 仅转调 `CreateVersion`；`router.go` 未挂载，仅 `coverage_push_test.go:69` 引用。
- **动作**：删除方法与测试路由。
- **验证**：`go test ./internal/api/`。

### S3. `observability/redact-desens.go` 的 `RedactString` / `RedactMap`
- **证据**：非测试引用 0 处（启动日志脱敏走 `config.Redacted()`，S3 走 `objectstore.redactor`）。文件名 `-desens` 后缀系脱敏工具误产物。
- **动作**（二选一，默认 a）：
  - a. 删除文件与 `observability_test.go` 中对应用例；
  - b. 若希望日志统一脱敏，则在 `observability.Logger` 字段层接入并改名 `redact.go`（属功能变更，另立项）。
- **验证**：`go test ./internal/observability/`。

### S4. LLM sessions 读 CRUD 路由（原 cleanup-plan D2，本轮提级）
- **证据**：`router.go:348-354` 六条 `/llm/sessions*`；UI、JS-SDK、Go-SDK、docs **全部 0 消费**（2026-10-06 grep）。写入侧 `llm_handler.persistChat` → `CreateLLMSession` 仍活跃。
- **动作**：删除 6 条路由 + `system_handlers.go` 中 `llmSessionHandler`（List/Create/Get/Delete/ListMessages/PostMessage）+ 对应测试；`systemdb/llm.go` 中仅被这些 handler 使用的 `ListLLMSessions/GetLLMSession/ArchiveLLMSession` 等一并删除（先 grep 确认 cloudagent 不用）。
- **保守选项**：若担心外部调用方，先只摘路由保留 store 方法，下一版再删。
- **验证**：`go test ./internal/api/ ./internal/systemdb/`。

### S5. `internal/api/zz_repro_test.go`
- **证据**：命名 `TestReproExact`，为一次性问题复现用例，场景已被 `cronjob_handler_test.go` 覆盖。
- **动作**：确认覆盖后删除；若有独特断言则并入 `cronjob_handler_test.go`。

### S6. `/llm/providers` 与 `/audit` 只读端点（降级为决策，见 D1）
- 仅列出证据：UI/SDK/docs 0 消费；但属运维面，**默认保留**。

---

## M. 后端重复代码合并

### M1. `limit` / `cursor` 查询参数解析（6 处各写一遍）
- **位置**：`system_handlers.go:53`、`users_handler.go:62`、`cronjob_handler.go:370`、`sandbox_handler.go:108`、`database_handler.go:355`、`handlers_plan79.go:94`。
- **动作**：在 `internal/api/context.go`（或新建 `params.go`）加 `queryLimit(c echo.Context, def, max int) int`，6 处替换。各处默认值/上限不同，作为参数传入，行为不变。
- **验证**：`go test ./internal/api/`（各 handler 已有 limit 边界用例）。

### M2. JSON 请求体解码（10 处 `json.NewDecoder(c.Request().Body)` + 部分 `DisallowUnknownFields`）
- **位置**：`data_handler.go`×3、`sql_handler.go`×3、`sandbox_handler.go`、`database_handler.go`、`api_keys_handler.go`、`projects_handler.go`。
- **动作**：抽 `decodeJSONBody(c, dst any, strict bool) error`，统一返回 `ErrInvalidRequest`；`sql_handler` 的请求体上限逻辑保持在调用方（`http.MaxBytesReader` 先行）。
- **注意**：逐处对比错误码，确保响应 code 不变（有测试断言 `invalid_request`）。

### M3. `internal/log` 并入 `internal/observability`（原 D3，本轮执行）
- **证据**：`internal/log` 唯一 importer 是 `observability/logger.go`，形成「245 行 zap 封装 + 70 行门面」两层。
- **动作**：`git mv internal/log/log.go internal/observability/zaplog.go`（包名改 observability，未导出化内部符号），测试同迁；删除 `internal/log/`；更新 `AGENTS.md`/`internal/README.md` 模块清单。
- **验证**：`go build ./... && go test ./internal/observability/`。

### M4. `*_plan79.go` 文件按领域重命名（纯命名合并，无逻辑改动）
- `adapters_plan79.go` → 内容并入 `adapter.go`（同为 Dependencies 适配器，合并后约 325 行）；
- `handlers_plan79.go`（Quota + Audit handler）→ 重命名 `quota_audit_handler.go`；`handlers_plan79_test.go` 同步。
- **验证**：`go build ./... && go test ./internal/api/`。

### M5. `stage_timing_lease.go` 并入 `stage_timing.go`
- 103 行，仅服务 stage timing；合并后单文件约 400 行，便于将来整体退役（见 D3）。

---

## U. 前端死代码删除

### U1. `components/CronJobRunsDrawer.vue`（211 行）
- **证据**：业务代码 0 引用；`CronJobs.vue:210` 实际使用 `modal/CronJobRunsModal.vue`（同功能 SbModal 版，代码几乎逐行相同）。仅 `tests/CronJobRunsDrawer.test.ts` 引用。
- **动作**：删组件 + 测试；确认 `CronJobRunsModal.test.ts` 覆盖率不降。

### U2. `Api` 契约中业务零消费的方法（契约 + http-api + mock 三处）
2026-10-06 逐项扫描（排除 `services/`、`test/`）：

| 方法 | 后端路由 | 建议 |
| --- | --- | --- |
| `gofunctions` 全部活跃 | — | — |
| `users.get`、`users.remove` | 有 | **删**（Users 页仅 list/create/update，删除用户走禁用） |
| `sandboxes.get`、`sandboxes.update` | 有 | **删**（Sandboxes 页用 list + 操作返回值刷新） |
| `cronjobs.get` | 有 | **删** |
| `databases.get` | 有 | **删** |
| `db.update` | 有 | 保留（文档编辑能力预留，DocumentKvModal 计划支持编辑）——若确认不做则删 |
| `agents.get`、`agentThreads.get`、`agentThreads.remove` | 有 | `agents.get`/`agentThreads.get` **删**；`agentThreads.remove` 建议补 UI 入口（会话删除是合理诉求）而非删除 |

- **动作**：每删一个方法同步删 `types.ts` 字段、`http-api.ts` 实现、`mock.js` 方法、`src/test/api-mock.ts` 桩与 `tests/http-api*.test.ts`/`mock.test.ts` 对应用例。后端路由**不动**（SDK 与外部调用仍可用）。
- **验证**：`yarn typecheck && yarn build && yarn test`。

### U3. `ProjectScope.vue` 等低引用组件复核
- 已核实活跃（9 处引用），**不删**，记录在 K。

### U4. `lib/status.ts` 导出面收窄
- `logLevelTextMap`、`statusTextMap` 外部 0 消费 → 改为非导出常量（顺手项）。

---

## V. 前端重复代码合并

### V1. 错误消息提取（91 处 `e instanceof Error ? e.message : '...'`）
- **动作**：`utils/format.ts` 新增 `errorMessage(e: unknown, fallback: string): string`，逐页面替换。纯机械替换，分页面 commit。
- **验证**：`yarn build && yarn test`。

### V2. 时间格式化重复（3 处私有实现）
- `AgentScheduleModal.vue:307 formatTime`（附加 ` UTC` 后缀，疑似 bug：`toLocaleString` 已是本地时间）、`UserFormModal.vue:129 fmtTime`、`Users.vue:175 fmtTime` 与 `utils/format.ts formatTime` 重复。
- **动作**：统一用 `utils/format.ts` 的 `formatTime`；如需 `—` 占位则给 `formatTime` 增加可选 `empty` 参数；`shortTime` 移入 `format.ts`。删除 ` UTC` 误标（需与产品确认）。

### V3. 状态 Badge 映射分散
- `AgentScheduleModal.vue:300 statusVariant` 与 `lib/status.ts` 的 `cronStatusText` 等同构。
- **动作**：在 `lib/status.ts` 增 `runStatusVariant`（completed/failed/running/queued），AgentSchedule 与 CronJob 运行记录共用。

### V4. `http-api.ts` 两套 SSE 读取循环
- `llmStream`（119 行起）与 `streamAgentRun`（470 行起）各自实现 `fetch` → `getReader` → `TextDecoder` → 按 `\n\n` 切帧 → 解析 `data:` 的完全相同循环。
- **动作**：抽私有 `readSse(resp, onEvent(event, data))`，两处只保留事件分派；鉴权头、401 处理保持原样。
- **验证**：`tests/http-api.test.ts` 中 stream 相关用例全绿；手测 AgentManager 对话与 AiChat 流式。

### V5. `useAsyncAction` 推广或删除
- **证据**：仅 `Databases.vue` 使用；其余 18 个文件手写 `loading = ref(false)` + try/finally。
- **动作**（二选一，默认 a）：
  - a. 在 V1 替换同时，将页面级 `load()` 改用 `useAsyncAction`（loading/error 统一）；
  - b. 若不推广，则把 Databases.vue 改回手写并删除该 composable（减少一种模式）。
- 默认 a，但**按页面单独 commit**，不与 V1 混在同一 commit。

---

## T. 测试文件归并（原 ui-cleanup-plan J3，本轮执行）

### T1. 前端「凑覆盖率拼盘」归并
- `coverage-gaps`(395) / `low-branch-coverage`(173) / `pages`(152) / `pages-coverage`(380) / `pages-interactions`(232) / `simple-components`(111) / `components-coverage`(199) / `components-interactions`(325) 共 8 个文件约 2000 行，按被测对象拆回 `<Component>.test.ts`。
- KV 测试 10 个文件（约 2400 行：`KvBlind/KvBranches/KvCallbacks/KvKeydown/KvTemplate/...`）按 `KvPanel` / `KvEditors` / `KvModals` / `KvApiPanel` 合并为 4 个。
- **约束**：合并前后 `yarn test --coverage` 语句/分支覆盖率不得下降；先复制用例到目标文件 → 跑绿 → 再删源文件。
- **预计**：93 → 约 75 个测试文件。

### T2. 后端 `coverage_*_test.go` 拼盘
- `api/coverage_push_test.go`、`systemdb/coverage_{agents,data,store}_more_test.go`、`app/more_test.go` 等按被测文件归位（S2/R5 改动会先触及其中部分用例）。
- 低优先级，可在 S/M 组完成后执行。

---

## D. 需决策项（默认本轮不动）

| 编号 | 项 | 现状 | 建议 |
| --- | --- | --- | --- |
| D1 | `/llm/providers`、`/audit`、`/quota` 端点 | providers/audit 前端与 SDK 0 消费；quota 仅 Dashboard 一个状态卡 | 保留（运维/外部可用）；在 docs 补说明或下版收紧 |
| D2 | `services/mock.js`(1289) + `mock-kv.js`(943) 退役 | 每个新 API 要三处维护；三套 env 默认 `false`；仅 2 页显示 Mock 徽标 | **强烈建议本版决策**：退役可删 ~2200 行 + `mock*.test.ts`，`api.ts` 固定导出 httpApi，并更新 `ui/AGENTS.md` 约定。代价：失去无后端演示 |
| D3 | perfbench + stage_timing 体系 | `internal/api/perfbench` 1583 行，零生产耦合 | 保留（DuckLake 延迟回归唯一工具） |
| D4 | `/v1/projects/:p/settings` 与 `/v1/settings` | UI 只用 `/llm/settings`，通用 settings 端点 0 消费 | 确认外部是否使用后再收紧 |
| D5 | `examples/lib/goexample` 与 `examples/shared/goexample/cmd/build` 双目录 | shared 仅剩一个 build 命令 | 合并到 `examples/lib/goexample/cmd/build`，同步 README 中的 `go run` 路径 |

---

## K. 防误删清单（本轮复核）

| 项 | 证据 |
| --- | --- |
| `sqlguard/classifier-desens.go` | 文件名带 `-desens` 但为 sqlguard 核心实现，**活跃**；仅可选改名 `classifier.go` |
| `sandbox/manager.go`、`driver*.go`、`paths.go` | app.sandboxAdapter + sandbox_handler 在用 |
| `api/kv_commands.go`(1426) | KV 单端点核心，46 个 Repo 方法全被调用 |
| `ProjectScope.vue`、`AiChat`、`AgentScheduleModal` | 9 处 / AgentManager 在用 |
| `CronJobRunsModal.vue` | CronJobs 页在用（U1 删的是 Drawer 版） |
| `composables/usePagination.ts` + `TablePager` | 7 页在用 |
| `editor/goMonarch.ts`、`databases/kv/kv-endpoints.ts` | 经 import 链消费（basename grep 计 0 属误判） |
| `isMock` 徽标 | 若 D2 退役 mock，则一并删除 DefaultLayout/Dashboard 徽标与 S3Manager 上限分支 |
| `systemdb` 全部 `sys_*` DDL | 迁移历史保留 |
| `db.update` 契约 | U2 中保留项 |

---

## 执行顺序

1. **前置**：提交现有工作区改动（sandbox / docs / examples 重组）。
2. **R 组**：R1→R2→R3→R4（独立 commit）；R5 与 S 组一起；R6/R7 放最后与文档一起。
3. **S 组**（每项独立 commit）：S2 → S5 → S3 → S1 → R5 → S4。
4. **M 组**：M4（纯改名，先做降低后续冲突）→ M5 → M3 → M1 → M2。
5. **U 组**：U1 → U4 → U2（按 Api 分组逐个 commit）。
6. **V 组**：V2 → V3 → V4 → V1（按页面分批）→ V5。
7. **T 组**：T1（KV 先，拼盘后）→ T2。
8. **文档**：R6、R7、`internal/README.md`、`internal/AGENTS.md`（log 并入、sandbox Client 移除）、`ui/AGENTS.md`（若 D2 决策退役 mock）。

## 验证命令

```bash
# 后端（每个 commit）
go build ./... && go vet ./...
go test ./internal/... -count=1
go test ./gofunction/... -short

# 前端（每个 commit）
cd ui && yarn typecheck && yarn build && yarn test

# SDK（S4/U2 后确认未受影响）
cd packages/js-sdk && npm run typecheck && npm test

# 残留检查
git ls-files | grep -E "gofunction\.test$|^datasets/|^output/cfbee|^skills/"          # 应为空
grep -rn "sandbox\.New(\|ThreadSandbox" internal --include='*.go'                    # S1 后应为空
grep -rn "llm/sessions" internal/api/router.go                                        # S4 后应为空
grep -rn "internal/log\"" --include='*.go' .                                          # M3 后应为空
grep -rn "instanceof Error ? e.message" ui/src | wc -l                                # V1 后应接近 0
grep -rn "CronJobRunsDrawer" ui/                                                      # U1 后应为空
```

## 风险与回滚

- 所有删除项均为「生产代码 0 引用」，协议层无字段变化；后端路由仅 S4 一处删除（0 消费方，需在发布说明标注）。
- M2 JSON 解码合并是唯一可能改变错误响应的项——逐处比对现有测试断言，错误码保持 `invalid_request`。
- V2 去掉 ` UTC` 后缀属可见文案变化，需确认。
- T 组只搬迁用例，以覆盖率不降为闸门。
- 每项独立 commit，按 commit revert；不动 `go.mod`、`package.json`、`tsconfig.json`、`vite.config.ts`（R1 仅改 `.gitignore`，已在旧计划授权范围内）。

## 修订记录

- **v1.0**（2026-10-06）：初版。汇总 cleanup-plan 遗留 7 项；新增 sandbox 旧 Client、GoFunction Update、Redact 死代码、LLM sessions 读路由；后端 limit/JSON 解码/log 包/plan79 文件合并；前端 CronJobRunsDrawer、9 个零消费 Api 方法、错误消息/时间格式/SSE 循环合并；测试拼盘归并。
