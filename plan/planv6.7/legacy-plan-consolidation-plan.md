# v1.0–v6.4 历史计划清算：仍有效待办与契约锚点（planv6.7）

> 方法：逐份通读 `plan/planv1.0` … `plan/planv6.4` 共 **107 份文档**（约 3.8 万行），
> 对每条「未完成 / 待办 / 后续」逐条回代码核实（`rg` 定位到文件与符号），再决定：搬运、作废或丢弃。
> 本轮**只写本文**，源码零改动。
>
> 承接：`planv6.3/backend-cleanup-simplify-plan.md`、`planv6.6/internal-simplify-code-plan.md`
> （后者已把 B1–B4 批次执行完，本轮未重复）。
>
> **代码注释里的 `planv1.0`–`planv6.4` 编号（`planv4.1 BUG-0X`、`planv6.0 D5`、`planv6.0 v2 §2.2`、
> `planv6.1 O2`、`planv6.2 §6-2`、`planv6.4 P0b`、`production-readiness-plan §8.x` 等）
> 一律查本文 §2 的锚点表**——旧目录删除后，这里就是唯一落点。

## 0. 结论摘要

| 轮次 | 文档数 | 已完成 / 已被代码取代 | 仍有待办 | 纯历史可丢 |
| --- | --- | --- | --- | --- |
| v1.0–v5.0 | 71 | 39 | 12 份（21 条，§3 A–F） | 12 |
| v6.0–v6.4 | 36 | 27 | 9 份（35 条，§3 G–K） | 4 |

真正的「未完成」合计 **60 条**，分 11 组（§3 A–K）。**旧文档里已被后续版本修掉或写错的结论另有 25 条**，集中在 §5 作废清单——这是本次检查最容易被忽略的产出：不做反向清单，后来者会按旧文档把已修的缺陷再"修"一遍，甚至删掉活代码（`NewEvictor`/`OrphanLedger` 就是活例子）。

两轮共同的最大发现：**多数文档的自述状态已失真**。`production-readiness-plan` 写「规划（未实施）」但 M1/M1.5/M4 已在代码里；`planv6.3/console-data-trigger-ops-fix` 写「待实施」实际已完成；`planv6.0/openai-compatible` 写「待确认后实施」实际已上线并超出计划。**判定一律以代码为准，不采信文档自述**。

## 1. 处置总表

### 1.1 planv1.0（14 份 · Turso/libSQL 时代）

| 文档 | 处置 | 说明 |
| --- | --- | --- |
| `plan.md` | 抽 1 条原则 | §2.2 单写论证（副本数=1、Recreate 升级、启动 instance identity 冲突检测"不能宣称分布式锁"）；状态机与 §5.4「S3 故障不得静默返回持久化成功」仍有效 → §4.1 |
| `plan1`–`plan6`、`plan8`–`plan10` | 丢弃 | 装配顺序、S3 键布局、catalog 状态机、sqlguard 黑名单、LLM SSE 规则均已被 `internal/` 代码与 `AGENTS.md` 承接；`internal/database/turso`、`internal/deploy`、`internal/jobs` 已不存在 |
| `plan7` | 丢弃 | 缓存/恢复点/jobs 整包已删；仅「不自研复制活跃 `.db/.db-wal`」一句结论成立 |
| `plan11` | 丢弃 | UI 联调记录；`/data/*` 已迁到带 `databaseId` 的路径 |
| `ui-api-contract.md` | 丢弃 | 文件首行自述「已被 planv2.0/proto-http.md 取代」，路径体系（`/api`、`/db`、`/faas`）全不符 |
| `ui-refactor-plan.md` | 抽 1 条 | 「二次确认 + 空态 CTA + loading 三选一」交互原则；antd 配色全部作废 |

### 1.2 planv2.0（26 份）

| 文档 | 处置 | 说明 |
| --- | --- | --- |
| `proto-http.md` | **丢弃 + 指向代码** | 2029 行、**正文被重复拼接 4 份**且首份在 487 行截断。路由/错误码以 `internal/api/router.go` + `internal/api/error.go` 的 `sentinelErrorMappings` 为准 → §4.2 |
| `db-ducklake-plan.md` | 抽参数表 | 数值最密：DuckDB ≥1.5.2、retry 10/100ms/1.5、压缩分层 1→5→32MB、debounce 200ms、`allowed_paths` 一票否决 → §4.3 |
| `databases-and-s3-plan.md` | 抽边界规则 | 平面 A/B 分离、`durability ∈ {committed_local, synced_s3}`、descriptor 旧格式一律拒开 → §4.3。**文档自带矛盾**（文首"不做 MinIO e2e" vs §2.2/§4.5"必做"）不再继承 |
| `config-yaml-unification-plan.md` | 视为已落地 | `config.example.yaml`、`rejectYAMLSecrets`、`internal/config/yaml.go` 均在；"配置只从 yaml + env 加载"已写进 `AGENTS.md` 硬性约束 2 |
| `system-database-plan.md` | Phase C/D 进 §3-D3 | Phase A/B 已落地；`llm` stream 未落库、`provider-catalog` 未做、日志保留任务未做、前端未接系统 API |
| `cloud-agent-plan.md` + `agent-schedule-plan.md` | 抽安全边界 | Prompt 7 段组装 + 永不进 prompt 清单 + 一次 redact；内置只读 principal + CAS 认领（cronjob 沿用）→ §4.4。**`/agent-schedules` 已整体下线**，路由部分作废 |
| `ui-cronjob-plan.md` | 抽契约 | `cron` vs `interval` 语义、截断策略（error 300 / response 4KB / input 8KB）、合法域 60s～30天 → §4.5 |
| `ui-gofunction-plan.md` | 抽签名契约 | `func Name(req T) R`、`package main`、`/go/...` 必须注册在 SPA fallback 之前 → §4.5 |
| `ui-databases-console-plan.md` | 抽硬约束 | 「调用方已给 `databaseId` 时禁止静默落第一个库」→ §4.5 |
| `ui-settings-chat-plan.md` | 抽厂商表 | 10 家预置厂商的 id/协议/默认模型 → §4.6（设置入口已改为 Modal，实现部分作废） |
| `ui-polish-and-merge-plan.md` | 进 §3-E2 | 唯一对得上当前技术栈的 UI 待办；行号清单已部分过期（见 §3-E2 逐条核实结果） |
| `ui-style-plan.md` / `ui-principles.md` / `ui-plan-v2.md` / `ui-shadcn-migration-plan.md` / `ui-refactor-plan.md` | 丢弃 | antd 时代；迁移已完成（reka-ui 2.10.5 + Tailwind） |
| `ui-docs-wiki-plan.md` / `ui-global-project-plan.md` / `ui-settings-merge-plan.md` | 丢弃 | 已实现；`SbModal` 现位于 `ui/src/components/modal/SbModal.vue` |
| `cleanup-refactor-plan.md` | 抽防误删 | 保留 `sys_jobs` DDL（迁移历史）、`SetDatabaseDegraded`；「`CatalogConfig.DatabaseID` 退役属 breaking」仍待确认 |
| `js-sdk-plan.md` | 抽安全文案 | 「浏览器暴露 Key = 该 Key 全部权限；当前无行级 RLS」→ §4.6 |
| `gofunction-*`（5 份） | 抽基线 | P3-6 tagged union 仍是「以后」；`bench_report.md` 已承接数字 |

### 1.3 planv3.0（11 份）

| 文档 | 处置 | 说明 |
| --- | --- | --- |
| `multi-instance-consistency-plan.md` | 进 §3-C + §4.7 | §1.3 Parquet 三不变量、八条静默损坏路径、COS 不支持 `If-Match`、孤儿账本规则、split-brain Runbook 是全仓唯一 |
| `api-db-perf-validation-plan.md` | 进 §3-B | §1 九段耗时模型 + §2.4.4 预算标尺仍唯一；P4/P5/P6 未做 |
| `gofunction-versions-testplan.md` | 进 §3-D2 | 三表 DDL、10 端点×权限、test envelope、§14.4 七项 TODO 未关闭 |
| `key-value-ducklake-plan.md` | 抽语义表 | 「缺失 key ≠ 404」语义（`GET`→null、`TTL`→-2、`TYPE`→none、`DEL`→0、`SCAN`→`"0"`）→ §4.5；**"每项目一份 `kind=kv` catalog" 已作废**（现为 `godatabase/kv` + `__kv` 内部表） |
| `ducklake-duckdb-catalog-plan.md` | 抽协议 | 启动格式校验 + `reset` 五层防护，是唯一成文的「引擎切换/清库」协议 |
| `ducklake-storage-latency-optimization-v2.0-plan.md` | 部分作废 | §0-1「锁定 SQLite catalog」已被 `ducklake-duckdb-catalog-plan` 修订为「锁本地单文件 catalog、引擎可配置」——**以后者为准** |
| `login-auth-plan.md` | 丢弃 + 指向代码 | P1–P6 已落地；角色矩阵/argon2id 参数/JWT payload 以 `internal/auth` 为准。**「10 次/分→429」已过期**（现为三维度软锁定，见 `internal/api/login_guard.go`） |
| `cloud-agent-sandbox-plan.md` | 丢弃 | microsandbox Cloud 方案**从未实施**；现行沙盒是 `gosandbox` + `internal/sandbox`（local/fake） |
| `database-always-open-plan.md` | 丢弃 | 九态→四态的迁移已完成，`internal/database/registry` 已不存在 |
| `homepage-console-entry-plan.md` | 丢弃 | `/`、`/console`、`/docs` 结构已存在 |
| `ui-style-polish-plan.md` | 抽对比度 | `#d97757` vs `#faf9f5` = 2.96:1、vs `#f5f4ef` = 2.83:1，未达 4.5:1；候选 `#a84627` ≈5.34:1 → §4.8 |

### 1.4 planv4.0（11 份）

| 文档 | 处置 | 说明 |
| --- | --- | --- |
| `cloud-agent-optimization-plan.md` | **锚点全搬 §2.1** | 这是 `planv4.1 §X` / `BUG-0X` 的唯一载体；先落锚点再删目录 |
| `cloud-sandbox-plan.md` | 抽契约 | 10 条错误码、DDL/状态机、`run`/`files` 响应契约、五层限制、审计只记命令 SHA-256 → §4.9 |
| `ducklake-rw-latency-eventual-consistency-plan.md` | **进 §3-B** | P2-1…P2-4 全未做；`committed_local` + `sync_lag_seconds` + RPO 30s + 三开关是 §4.3 的一部分 |
| `examples-business-cases-plan.md` | **进 §3-D1** | G0b–G16 平台缺口 backlog 的唯一登记处（`examples/*/README.md` 7 处引用） |
| `console-polish-and-systemdb-perf-plan.md` | 抽 3 条契约 | 日志选择性记录规则、summary/trend/quota 短 TTL 单飞缓存、`WriteGate` 按库 `TryDo` + `ErrAcquiring`→503+Retry-After → §4.10 |
| `dashboard-api-latency-percentiles-plan.md` | 抽 4 行 | 桶边界 `1,2,5,10,…,30000,+Inf`、字段名、无样本返 `null`、`+Inf` 不入响应 → §4.10 |
| `cleanup-plan.md` | 抽防误删 | E 组 22 行防误删清单 + 「历史 plan 是注释锚点，不能乱删」这条规则本身 |
| `simplify-merge-plan.md` / `ui-cleanup-plan.md` | 抽防误删 | §K 防误删（`-desens` 文件名陷阱、深路径 import 浅 grep 计 0、`ui/dist` 是 go:embed 产物） |
| `IMPLEMENTATION_SUMMARY.md` | 抽 1 表 | 配置回滚开关表（`data_inlining_row_limit:0` / `catalog_sync.mode:debounce` / `lease.enabled:true`）；其余三段被各自 plan 反超 |
| `docs-site-polish-plan.md` | 进 §3-D5 | P2 三篇文档 + 人工验收未做 |

### 1.5 planv5.0（10 份）

| 文档 | 处置 | 说明 |
| --- | --- | --- |
| `backend-optimization-v3-plan.md` | **大部分已作废** | SEC-01/02/03、RACE-01、WR-01、SYNC-01 均已被 v6.0 系列修掉（证据见 §5） |
| `system-database-sql-tables-plan.md` | 抽安全规范 | 6 条敏感列名单 + 两层脱敏 + `rows` 契约是**不可替代的安全规范** → §4.11 |
| `db-read-performance-and-s3-architecture-plan.md` | 进 §3-B + §4.3 | 能力边界、计量纪律（全库文件数≠扫描文件数）、官方机制对照表 |
| `db-read-performance-optimization-v2-plan.md` | 抽基线数字 | 作为计划已被上一条吸收；`principalMu` convoy 已修（见 §5） |
| `console-loading-states-plan.md` | 抽状态表 | 七状态判定表 + 160ms 出现延迟是唯一权威；`SbAsyncRegion`/`SbErrorState`/`SbTabsPanel` 均已落地 |
| `ui-optimization-v3-plan.md` | 抽 i18n 分层 | i18n 已落地（`ui/src/locales/`），但 reka `data-active` 纠正与覆盖率陷阱仍要引用 → §4.8 |
| `code-and-style-optimization-plan.md` | **大部分已作废** | typecheck 已清零（planv6.6 OPT-4）、KV `EXISTS` 已修、`data-active` 判断被 v5.0 后续纠正 |
| `cloud-assistant-skills-cli-plan.md` | 抽 CLI 基线 | 信封 `{"ok":true,"data":…}`、退出码 0/1/2/3/4、错误码→应对表、委托 token 三约束 → §4.12。文件首行已自标「由 planv6.0 修订」 |
| `cloud-assistant-sql-database-tabs-plan.md` | 丢弃 | 已实现；且「系统库恒非 SQL」已被推翻（现返回 `sql`） |
| `console-table-center-agents-entry-plan.md` | 抽 2 条踩坑 | tailwind-merge 会盖掉组件默认 `text-right`；`text-center` 只居中行内内容，需 `[&>.flex]:justify-center` |

## 1.6–1.10 planv6.0–v6.4（第二轮）

### 1.6 planv6.0（11 份）

| 文档 | 处置 | 说明 |
| --- | --- | --- |
| `cloud-assistant-chat-ux-plan-v2.md` | **锚点必须搬** | BUG-B1–B5 被 5 处代码注释引用（`planv6.0 v2 §2.2`）；双层校验顺序（skill 前缀 → 写门禁 → 破坏性确认 → headless）→ §2.5 |
| `gocloudagent-cli-and-cronjob-unification-plan.md` | **锚点必须搬** | 1605 行阶段表：A/B/C0/D/UI/F 已落地，C1–C4、E1–E5、E7 未落地；v49 DDL、16KiB 提示词上限、Q1–Q8 开放问题 → §2.5 + §3-G |
| `storage-local-first-plan.md` | 抽 P0 风险 | `cache.Manager.Evict` 按 LRU `removeAll` 唯一副本 → `data_dir` 与 `cache_dir` 必须物理分开；`identity.json` 存储身份契约 → §4.13 |
| `bugfix-cloud-agent-cli-chain.md` | 抽缺陷表 | §2 缺陷表 + 安全日志边界；**§5 的 `build.sh` 描述已与现状不符**（现为 dev 下 `go run` 找不到同目录 CLI） |
| `cloud-agent-module-mentions-and-thread-routing-plan.md` | 抽模块表 + 标注反转 | §3.1 九模块稳定 ID 表、角色/委托边界 → §2.5；**`@技能` 语义已被反转**（现由 `SkillsForModule` 决定，见 §5） |
| `cloud-assistant-skills-cli-v2-plan.md` | 抽决策表 | D1–D6 + SEC-03 教训（白名单字符串表与真实路由漂移、测试用虚构路径所以 CI 没抓到）→ §2.5 |
| `backend-code-simplification-plan.md` | 抽清单 | §1「必须保持不变」（导出符号/路由/权限/中间件顺序/错误码文本）、§2 末段「刻意不统一」；C4 从未执行 → §3-K |
| `frontend-code-simplification-plan.md` | 抽清单 | §1.1 Vue 响应式身份/数组复制/watch 时机/toast 次数/分页重置/SSE 游标；C1 `|| 0` NaN 语义、C3 竞态守卫 → §4.8 |
| `openai-compatible-llm-api-plan.md` | 抽约束 | §7 八条硬约束（双 `/v1` 由来、OAI 错误形状是 `WriteError` 唯一例外且限定 `oa` 组、不引 SDK 依赖手写 JSON）→ §4.2 |
| `cloud-assistant-chat-ux-plan.md`（v1） | 抽 3 条根因 | `push(reply)` 后改原对象不触发渲染、`prefers-reduced-motion` 把 `animate-spin` 压到 0.01ms、`useLoadState.emptyOn` 契约 → §4.8 |
| `console-s3-preview-docs-order-plan.md` | 丢弃 + 1 行 | 唯一值得留：后端 `ErrorRate` 是 0~1 比值而 UI 直接拼 `%`（20% 显示成 0.2%）→ §4.10 |

### 1.7 planv6.1（7 份）

| 文档 | 处置 | 说明 |
| --- | --- | --- |
| `gocloudagent-smoke-report.md` | **必须抽数字** | 全仓唯一的模型基线：MBPP 348/427（81.5%）、SWE Verified 5/10、TB 0/8、MBPP 13353s / ¥2.78、外推 500 题 ≈25.6h / ¥35、失败分类（断言 60 / 超时 15 / 运行错误 4）、跑法参数 → §4.14 |
| `gocloudagent-agent-benchmark-plan.md` | 进 §3-G | M1–M3 已落地（`gocloudagent/bench/*.py`），M4 `workflow_dispatch` 与 M5 扩展层未做；**口径更正：计划写 SWE-bench Lite 20，实跑是 Verified 10/50** |
| `gocloudagent-optimization-plan.md` | 进 §3-G + §4.14 | O1–O5 主体已实施；#5 `max_tokens` 可配、#8 停止判据、#10 L3 钻头用例仍缺；§1.1–1.4 三条修正（`litellm.ProviderConfig.Timeout` 从不被读、4096 隐性兜底、默认值分叉） |
| `gocloudagent-harness-supervisor-plan.md` | 进 §3-G | **从未实施**（全仓 `supervisor|advisor` 0 命中）；P0 部分被别的 PR 覆盖（`repeat_detector.go`、`budget_test.go`） |
| `examples-experience-fix-plan.md` | 抽 #8 决策 | #1–#9 已落地；唯一遗留「认证（user/apikey）进 skill」→ §3-J6 |
| `examples-automation-report.md` | 丢弃 + 1 条 | 63 步 62 通过；§7 第 8 条「认证不在任何 skill」与 J6 同源 |
| `docs-six-modules-consolidation-plan.md` | 丢弃 | 已完全体现为 `docs/_meta.json` + `ui/src/docs/catalog.ts` + 测试 |

### 1.8 planv6.2（7 份）

| 文档 | 处置 | 说明 |
| --- | --- | --- |
| `gocloudagent-mcp-plan.md` | **锚点必须搬** | 15 处代码注释引用 `planv6.2 §3.2/§4.1/§4.2/§4.3/§5.1/§5.2/§6-2/§6-4/§6-7` → §2.5；M0（SDK 契约探针）未留痕 |
| `sandbox-e2b-terminal-plan.md` | 进 §3-I + 抽契约 | M1–M6 落地、M0/M7 未做：**`docs/automation/sandbox-api.md` 仍写旧 `/v1/projects/:projectID/sandboxes`**、`sandbox.md` 仍写「没有 API Key」、`docs/` 全目录 `E2B` 0 命中；§4.6 与 E2B 的差异清单（本机 bwrap 非 Firecracker、无网络/模板/内存快照）唯一 |
| `project-single-file-database-plan.md` | 进 §3-K + 抽验收 | 阶段 A–E 已完成；**唯一架构偏离：`sys_databases.storage_prefix` 与 `Database.StoragePrefix` 未删**（仍作删除退路）；§11 验收 7 条 |
| `gosandbox-refactor-plan.md` | 抽边界规则 | M0–M7 全落地；§8 边界（`internal/gocloudagent` 只能 import `gosandbox/host`；隔离开启但不可用一律 fail-closed `isolation_unavailable`）+ §9 与调研的 6 条差异 → §4.15 |
| `gocloudagent-module-refactor-plan.md` | 抽阻塞分析 | 已完成；§2「哪些宿主类型无法靠换 import 解决」（`auth.Principal`/`systemdb.Store`/`sbcli.CommandReadOnlyWithArgv`/`sqlguard`/`llmgateway` 哨兵）仍是唯一论证 |
| `internal-utils-extraction-plan.md` | 丢弃 + 抽准入 | 已完成且被 v6.3 进一步搬移；§1 四条准入（「语义唯一，不许用参数开关糊」+ `idgen` 建了又删的教训）值得留在 `internal/utils/README.md` |
| `sandbox-research.md` | 抽 5 段 | 调研快照已过期（§5 实测方案被 gosandbox §9 的 6 条差异替换）；保留威胁模型、大厂逃逸 CVE、性能基线 0.277/2.313ms、Task 6 未做项 |

### 1.9 planv6.3（5 份）

| 文档 | 处置 | 说明 |
| --- | --- | --- |
| `backend-cleanup-simplify-plan.md` | 进 §3-K | A/B/C 批次全部真实落地；**N1 云函数单一路径是本文独有未完成项**（`systemDBRunner` + `runSem` 双分支仍在）；N8 剩余两个预留 API；S1 `api/perfbench` 未挪 |
| `godatabase-extraction-plan.md` | 抽契约 | 主体完成；G-2/G-3（`docker build`、`./scripts/smoke.sh`、端到端冒烟未执行）、G-4 覆盖率基线缺失；typed-nil Logger 陷阱与 `errors.Is` 别名契约 → §4.16 |
| `pocketbase-capability-gap-plan.md` | 进 §3-I | P0/P1/P2 主体已补；6 项真缺（relation/expand、索引 API、schema 导入 diff、集合重命名、每集合限流、SMTP）+ 12 项明确不做 → §4.17 |
| `sandbox-runtime-capability-gap-plan.md` | 进 §3-I | P0 三项落地；P1-1 违规监控、P1-2 祖先 pin、P1-3 `--args fd`、P2-1…P2-5 全未做；`gitdir:` 跟随遗漏；Linux 真 bwrap 用例未跑 |
| `console-data-trigger-ops-fix-plan.md` | 丢弃 + 2 条 | 实际已完成（自述「待实施」失真）；仅四视口人工验收与现场数据核查无法从仓库判定 |

### 1.10 planv6.4（6 份）

| 文档 | 处置 | 说明 |
| --- | --- | --- |
| `examples-skill-verification-plan.md` | 并入下面 report | 本体完成；§9.3「BUG-1/BUG-2 只记录不修」已过时（report §13/§14 已修） |
| `examples-skill-verification-report.md` | **整表必须搬** | `AGENTS.md` 点名的事实来源：**E1–E16 + BUG-0~5** → §2.5；`examples/README.md:11` 直链本文件 |
| `production-readiness-plan.md` | **进 §3-H + 锚点** | 自述「规划未实施」**严重失真**：M1/M1.5/M4 已落地、M3 只做登录没做授权；**20+ 处代码注释以 `§8.x` 为锚点** → §2.5 |
| `godatabase-optimization-plan.md` | 抽基线 | 全部完成；§8 性能表（`TestReadLatencyPhases` p50/p95/p99、`SingleWriteCost` 2.75ms/1393 newbytes）+ R3「DuckDB 快照可能不一致」降 P2 仅留回归用例 → §4.18 |
| `gofunction-optimization-plan.md` | 抽编号 + 偏差 | 全部批次完成；**A4 未执行**（binop 慢路径）、**验收线未达**（`ExecFib18` 1.04ms vs 0.9ms，差 15%）；标题写 v6.5 却放在 planv6.4/ → §3-K |
| `local-dev-cloud-replay-collab-plan.md` | 进 §3-J | P0a/P0b 已落地；P1 四项、P2 四项全未启动；§8 三条设计限制 → §3-J3/J4/J5 |

## 2. 引用锚点（旧编号 → 本文落点）

### 2.1 `planv4.1` = `planv4.0/cloud-agent-optimization-plan.md`

章节（删除后按此理解注释里的 `planv4.1 §X`）：

| 编号 | 含义 | 现状 |
| --- | --- | --- |
| §0 | 本版变化（相对 v4.0） | — |
| §1 | V1–V8 验证未通过的现象 | 已修 |
| §2 | BUG-01…BUG-12 清单与修复方案 | 全部落地（BUG-09/E17–E19 状态行未列，但代码与用例在） |
| §3 / §3.1–3.5 | 前端两列布局、`AgentCard.vue`、`ui/src/components/agent/` 组件族、`useAgentConversation` 状态机、视觉统一 | 已落地 |
| §4 / §4.1–4.6 | 通用 Agent、内置播种、SSE 补充、推理内容、重试与消息状态、模型列表 | 已落地；§4.4「思考正文」仍是已知限制（需升级 litellm，未获同意） |
| §5 / §5.1–5.5 | 测试方案（fakellm、用例 E01–E19、前端 vitest、真实模型冒烟、`scripts/`） | §5.4 R01–R12 真实模型冒烟未执行 |
| §6 | 配置变更（仅测试注入的内部参数不进 YAML） | 已落地 |
| §7 / §7.1 | 实施顺序 + v4.0 已完成项 | — |
| §8 / §9 | 验收 + 八条风险取舍 | — |

BUG 编号（注释里最高频）：

| 编号 | 一句话 | 落地位置 |
| --- | --- | --- |
| BUG-01 | `push(reply)` 后再改原始对象，不触发渲染 | `useAgentConversation.ts`（改用 `reactive`） |
| BUG-02 | thinking 心跳在工具执行期也发 | `streamRun` 引入 `phase`（model/tool） |
| BUG-03 | 重试重复落库 user 消息 | `retry_of_run_id` 校验失败 400 `invalid_retry` |
| BUG-04 | 删任意会话都停止当前运行 | 仅同 id 才 stop + `ConfirmAction` |
| BUG-05 | stop/error 后不刷新会话列表 | `onSettled` 统一刷新 |
| BUG-06 | assistant 消息不带 run 状态 | DTO 增加 `run_status` / `error_code` |
| BUG-07 | 前端拿不到模型列表 | `GET /v1/projects/:id/agents/models`（`database:read`），不返 `base_url`/`api_key` |
| BUG-08 | `call_id` 为空时本地补的 id 不回传 | 已修（`runtime.go`） |
| BUG-09 | tool_result SSE 截 2000 字但落库存全文 | 落库与 SSE 同上限 |
| BUG-10 | `stop()` 先关连接导致 metrics 不更新 | stop 后回填 `runs?limit=1` |
| BUG-11 | 已有 agent 时整体跳过播种 | `builtin_key` 唯一索引 + 按 module+name 幂等补齐 |
| BUG-12 | 运行中切换会话静默停止 | `ConfirmAction` 确认后再切 |

次级编号（若注释/提交信息出现，含义同上表）：`V1–V8`（§1 现象）、`E01–E19` / `B19–B28` / `S01–S03` / `A15–A18` / `U01–U24` / `R01–R12`（§5 用例 ID）。

### 2.2 `planv5.0` 被引用的章节

| 引用 | 原文含义 | 本文落点 |
| --- | --- | --- |
| `planv5.0 §2.2` | 401 被误当 Key 失效、匿名认证端点要排除 | §4.12「前端 401 处理」 |
| `planv5.0 §4.1` | reka `data-active` 命中、`data-[state=active]:` 统一写法 | §4.8 |
| `planv5.0 §4 P0.1` | pprof 与 `perf_stage_timing` 解耦、认证子阶段注入 | 已落地（`router.go` `mountObservabilityRoutes`） |
| `planv5.0 §7.2` | 前端覆盖率 functions 未达 95% | §3-E1 |
| `planv5.0 §7.3` | `helpers.ts` 把 `Switch` 桩成废弃 `checked` 契约导致假绿 | `ui/src/test/helpers.ts` 现用 `modelValue`，**已修** |
| `planv5.0 §8.2` | agent 写接口缺 `writable` 检查 | 已修（`writeGate`） |
| `planv5.0 §2.2`（http.ts 注释） | 匿名认证路径不吃 `markUnauthorized` | `ui/src/services/http.ts` `ANONYMOUS_AUTH_PATHS` |
| `planv5.0 §3.2` / `§3.4` / `§3.6` | 状态文案走 i18n、`SbErrorState`/`ConfirmAction` 语义、表单拆分 | §4.8；实现见 `ui/src/utils/agent-errors.ts`、`ui/src/components/SbErrorState.vue`、`useCronJobForm.ts` |

### 2.3 `planv2.0` / `planv3.0` 被引用的段落

| 引用 | 原文含义 | 本文落点 |
| --- | --- | --- |
| `plan/planv2.0/proto-http.md` | UI ↔ 后端 HTTP 契约 | §4.2（改指向 `router.go` / `error.go`） |
| `planv2.0/databases-and-s3-plan.md` | descriptor v2、平面 key 隔离、无 turso 用户库路径 | §4.3 |
| `planv3.0 列宽策略`（`ui/src/style.css`） | `sm/md/name/id/act` 五档列宽 | 已落地，注释保留即可 |
| `planv2.0/gofunction-perf-plan*` | 解释器 P0–P3 优化 | `gofunction/bench_report.md` 是现行基线 |

### 2.4 外部文件引用改法（删目录前必须改）

| 文件 | 现状 | 改成 |
| --- | --- | --- |
| `AGENTS.md`（设计计划文档段落） | 写「planv4.1 全文在 planv4.0/」 | 改为「planv1.0–v5.0 已删除，编号查 planv6.7 本文 §2」 |
| `AGENTS.md` ×4（`planv5.0 §7.2` / `planv4.1 §5.5` / `planv5.0 §4.1` / `planv5.0 §7.3`） | 裸编号 | 保留编号 + 指向本文 §2 |
| `examples/README.md`、`examples/{shop,jd,iot-telemetry,ops-assistant,community}/README.md` | markdown 链接 → `../plan/planv4.0/examples-business-cases-plan.md#6-…` | 改为本文 `#d1-examples-平台缺口` |
| `docs/workbench/agent-e2e.md` | 链接 → planv4.0/cloud-agent-optimization-plan.md | 改为本文 §2.1 |
| `gofunction/bench_report.md` | 链接 → planv2.0/gofunction-perf-plan*.md | 改为本文 §4.6 |
| `ui/src/services/types.ts` / `http-api-converters.ts` | 「契约对照 plan/planv2.0/proto-http.md」 | 改为本文 §4.2 |
| `internal/objectstore/plan_databases_s3_test.go` | 「对齐 plan/planv2.0/databases-and-s3-plan.md」 | 改为本文 §4.3 |
| `ui/src/lib/status.ts` | 「plan/planv5.0/ui-optimization-v3-plan.md §3.2」 | 改为本文 §4.8 |

其余约 30 处裸编号注释（`planv4.1 BUG-07`、`planv4.1 §5.1`、`planv4.0 cloud-sandbox-plan §4` 等）**不改代码**，统一由本文 §2 承接。

### 2.5 `planv6.0`–`planv6.4` 被引用的编号

#### 2.5.1 planv6.0（120 个文件引用，最高频）

| 引用 | 原文含义 | 本文落点 |
| --- | --- | --- |
| `planv6.0 D1` / `D2` | 委托白名单改路由表驱动；助手路由不可委托 | 已落地（`delegable()` + `delegation_route_table_test.go`） |
| `planv6.0 D3` | 宿主机文件隔离：助手模式 `--file`/`--init-sql` 只接受 `-`，内容走 `--content-base64` | `internal/sbcli/{commands,commands_database,commands_object}.go`、`internal/agentbridge/command.go` |
| `planv6.0 D4` | `users` skill 下线，`project` 只留 `quota get` / `audit list` | `gocloudagent/skills/simplebase/references/` |
| `planv6.0 D5` | `initiator_id`（UserID 或 `apikey:<id>`），确认/取消要求同发起人或 `project:admin` | `systemdb/agents.go:128`、`agent_approvals.go:53`、`cloudagent/delegation.go` |
| `planv6.0 D6` | 样例集改打真实 router（不再用桩服务器） | `api/agent_skills_router_e2e_test.go`、`agent_skills_e2e_test.go` |
| `planv6.0 §3.3`–`§3.6` | skills-cli-v2 §3：发起人校验 / gate 回收与等待 / 脱敏与 SQL 分类 / skill 精简与补全 | 代码注释里的「§3.5 脱敏规则」「§3.6 skill 对账」「§3.4 预登记」均指这里（`skills-cli-v2-plan.md`） |
| `planv6.0 §3.7` | **两个来源**：`skills-cli-v2` §3.7 = 样例打真实 router；`chat-ux-plan` §3.7 = 选中助手时不新建会话 | 代码里 `systemdb/agents.go:83`「每个助手一份会话」指**后者** |
| `planv6.0 §7 A3` / `§8 B2` | `backend-code-simplification-plan` 的批次项（A3 = CloudAgent 列顺序；B2 = 错误构造收敛） | `systemdb/agents.go:53` |
| `planv6.0 v2 §2.2` | **BUG-B1–B5**（见下表） | `api/run_events.go`、`agent_run_handler.go`、`ui/http-api-converters.ts` |
| `planv6.0 v2 §3.2` | Agent `writable` 列 + 迁移 v48 | `systemdb/agents.go:71` |
| `planv6.0 v2 §3.4` | 运行时双层校验 + `CommandReadOnly` / `CommandReadOnlyWithArgv` 只读分级 | `internal/sbcli/commands.go:45,94,100` |
| `planv6.0 v2 §3.5` | 提示词与 `writable` 一致性（capability 声明） | `cloudagent/prompt.go:28,42`、`modules.go:56` |
| `planv6.0 v2 §5` | 运行日志 `agent.trace_log`（默认关闭） | `config.go:332`、`app/assemble_agent.go:77` |
| `planv6.0 §8 B2`（cli-chain） | CLI 链路缺陷批次 | `bugfix-cloud-agent-cli-chain.md`（已修，见 §5） |

**BUG-B1–B5**（`cloud-assistant-chat-ux-plan-v2.md` §2.2，代码直接引用该节号）：

| 编号 | 一句话 |
| --- | --- |
| B1 | 续传缓存丢失时返回 200 空体（客户端无法区分"没数据"与"丢了"） |
| B2 | 子通道静默丢帧（`tool_result` 等非 token 帧被合并丢弃） |
| B3 | `truncated` 标记断链，前端不知道结果被截断 |
| B4 | `confirmHub.pending` 只增不减（内存泄漏） |
| B5 | 连接建立期无法续传（首帧前断开就丢 runID）→ 靠 `X-Run-Id` 响应头解决 |

**九模块稳定 ID**（`cloud-agent-module-mentions-and-thread-routing-plan.md` §3.1）：
`general` / `database` / `gofunction` / `cron` / `kv` / `s3` / `logs` / `sandbox` / `users`（`users` 未实现，见 §3-J6）。
**角色边界（反直觉，务必保留）**：`auth.RoleAdmin` 是**只读**管理员；只有 `superadminl1`（`RoleSuperAdmin`）能管用户；`project.isAdmin` 指**系统项目**不是用户角色；`/v1/users` 不可 `delegable`。

#### 2.5.2 planv6.1（14 个文件）

`planv6.1 O1`–`O5` 是 `gocloudagent-optimization-plan.md` 的优化项编号：

| 编号 | 内容 | 状态 |
| --- | --- | --- |
| O1 | CLI `end` 帧补 usage / model / duration | 已完成（`cli/agent_loop.go:63-70`） |
| O2 | litellm 重试与超时真传给 SDK（`MaxRetries=3`、`ConnectTimeout=10s`） | 已完成（`llmgateway/service.go:385-398`） |
| O3 | `finish_reason=length` → `ErrMaxTokensExceeded`，工具不执行 | 已完成（`agentcore/model.go:129-135`） |
| O4 | 预算与重复动作检测 | 已完成（`cli/budget_test.go`、`repeat_detector.go`） |
| O5 | 显式 `max_tokens` 可配 | **未做** → §3-G1 |

#### 2.5.3 planv6.2（35 个文件）

代码注释里的 `planv6.2 §X` 几乎都指 `gocloudagent-mcp-plan.md`：

| 引用 | 含义 | 落点 |
| --- | --- | --- |
| `§3.2` | MCP 工具命名规则（前缀/超长截断/冲突消歧/同名跳过） | `gocloudagent/mcp/naming.go` |
| `§4.1` | `MCPConnector` / `Runtime.MCP` 抽象：凭据与网络出口留在宿主，模块拿不到密钥 | `cloudagent/mcpaccess.go:12`、`runtime.go:75` |
| `§4.2` / `§4.2-1` | fail-closed 权限顺序（`Writable` 先于确认；未标记按写处理） | `cloudagent/mcptool.go:25`、`mcpaccess.go:21` |
| `§4.3` | 按 Agent 的 `mcp:` 引用装配工具 | `cloudagent/mcptool.go:175` |
| `§5` / `§5.1` / `§5.2` | CLI 侧集成：`~/.gocloudagent` 配置、环境剥离（过滤凭据类变量）、`mcp ...` 子命令 | `cli/{mcp_config,mcp_cmd,mcp_adapter,repl}.go` |
| `§6-2` / `§6-4` / `§6-7` | 管理端契约：凭据单向写、响应只回 `has_credential` | `internal/api/router.go:683`、`mcp_handler.go` |
| `planv6.2`（裸） | 模块拆分（`gosandbox` / `gocloudagent` 独立模块） | `gocloudagent-module-refactor-plan.md` |

#### 2.5.4 planv6.3（5 个文件）

`planv6.3` 裸引用 = `godatabase` 抽取 + `internal/dbbridge` 适配层；`N1`–`N9` 是 `backend-cleanup-simplify-plan.md` 的审批项（N2/N3 已被 planv6.6 Bug-2/Bug-3 覆盖，N4–N7 已被 T-1/T-2/T-5 与 T-6~T-9 覆盖，只有 N1/N8 仍开，见 §3-K）。

#### 2.5.5 planv6.4（20 个文件）

| 引用 | 含义 | 落点 |
| --- | --- | --- |
| `planv6.4 P0b` | `simplebase dev` 产品化进 CLI（`dev plan/apply/validate/status`、退出码 3、双端共用 `testdata/dev/*/expect.json`） | `internal/sbcli/dev.go:1`、`dev_workspace.go:1` |
| `planv6.4 §1` / `§2` | `godatabase-optimization-plan` 的批次（S/R/K/P/C） | `godatabase/{filedb,sqlguard}/*_test.go` |
| `planv6.4 §4.3` / `§4.4` | `local-dev-cloud-replay` 的验收与安全要求表 | `scripts/examples-verify/*`、`skills/simplebase/*` |
| `production-readiness-plan §8.x` | 生产就绪第一批细化的文件级方案（20+ 处注释锚点） | 见下表 |

**`§8.x` → 现状**（`internal/config|auth|api|app|systemdb|alerts|backup` 注释引用）：

| 锚点 | 内容 | 现状 |
| --- | --- | --- |
| §8.0 | `instance.profile`（单机/生产） | 已落地 `config/profile.go` |
| §8.1.3 | 安全响应头 | 已落地 `config.go:110` |
| §8.1.6 | `loginGuard` 三维度防护 | 已落地 `api/login_guard.go` |
| §8.1.7 | `/metrics` 收口（不上公网） | 已落地 `config.go:645` |
| §8.2.2 | 引导口令 + 强制改密 | 已落地 `auth/user.go:435`、`/v1/auth/password/initial` |
| §8.2.3 | 密钥域拆分（`SessionSecret` + 派生） | 已落地 |
| §8.2.4 | 版本化密钥环与轮换 | 已落地 `auth/keyring.go` |
| §8.2.5 / §8.3.3 | 审计补全 / `sys_audit_events` | 已落地 `systemdb/audit.go` |
| §8.2.6 | 终端用户域（`aud=enduser`） | 已落地 `auth/enduser.go`、`api/enduser_handler.go` |
| §8.3.1 | 日志落库 | **未验证** |
| §8.3.2 | 备份调度 + `missed_policy=catchup` | 已落地 `backup/scheduler.go` |
| §8.3.4 / §8.3.5 | 磁盘水位预警 / 告警总线 | 已落地 `alerts/{disk,bus}.go` |
| §8.3.6 | readiness 探针 | 只在内建 `app/doctor.go`，**无 HTTP 端点** → §3-H1 |

**E1–E16**（`examples-skill-verification-report.md`，`AGENTS.md` 点名的事实来源）：

| ID | 一句话 | 状态 |
| --- | --- | --- |
| E1 | `date +%s%3N` 是 GNU 扩展，BSD 下 `set -e` 首步崩 | 已修（`lib/common.sh now_ms()` 用 python3） |
| E2 | `PATH` 硬编码 nvm 路径 | 已修（`command -v node`） |
| E3 | `iot` 的 `once` 缺 `run_at`，validate 拒绝 | 已修（写死 `2099-01-01`） |
| E4 | agent cron 引用不存在的函数 | 已修（用例前 `function create`） |
| E5 | 默认 8080 被占时提示误导 | 已修（`--api-port`/`--skip-build`） |
| E6 | 清单不写 `data_model` 导致推断不一致 | 已修（4 清单显式声明） |
| E7 | 5 个遗留 `*-service` 目录含有效 Key | **部分**：目录已删，5 把 Key 无法吊销（旧实例不存在）→ §3-J2 |
| E8 | community 双库只写第一个库 | 已实现；BUG-0 后 community 单库，仅对多库场景有意义 |
| E9 | `resetSeeds` 只读 `001_init.sql` | 已修（遍历全部迁移） |
| E10 | `lib/goexample/cmd/build` 复制不存在的 `web/` | 已修（删除） |
| E11 | README 声称需服务端启用 CLI 技能，脚本不检查 | 已修（`run_agent.sh` 先 `GET /agents` 再决定 SKIP） |
| E12 | shop README 把同步调用写成 realtime | 已修 |
| E13 | 01/06/08 建的项目与函数不清理 | 已修（`PROJECT_ID_FILE` 复用 + `--fresh`） |
| E14 | 业务站点只有 README，无冒烟 | 已修（`scripts/examples-verify/smoke/*.sh`） |
| E15 | README 引用的 G0b–G16 需逐条复核 | **部分**：复核完成，13 个缺口仍未修 → §3-D1 |
| E16 | `.simplebase/` 不在 `.gitignore` | **未做**（决策性搁置）→ §3-J1 |

**BUG-0~5**（同报告）：

| ID | 一句话 | 状态 |
| --- | --- | --- |
| BUG-0 | 每项目只允许一个 `kind=user` 库，community 双库清单 409 | 样例侧已并库；平台约束保留 |
| BUG-1 | `dev apply --seed` 集合种子不幂等（服务端忽略传入 `id`） | 已修（`_migrations` 记账 `seed-doc:<集合>/<id>`） |
| BUG-2 | 删用户库 = purge 项目单文件，KV/备份一起丢 | 已修（`ProjectDataClearer`：只 DROP 用户表并清集合元数据） |
| BUG-3 | `init.sh` 换实例按旧 Key ID DELETE 遇 404 中断 | 已修（容忍 404） |
| BUG-4 | `/agent-schedules` 合并后 POST 返回 410 | 已修（改 `/cron-jobs` + `target_kind=agent`） |
| BUG-5 | CSV 声明 `text/csv` 嗅探成 `text/plain` → 415 | 已修（`plainTextSubtypes` 白名单） |

## 3. 仍然有效的待办

> 只列「回代码核实后确认仍未做」的项。每条写法：**现状证据 → 要做 → 验收**。
> 标注「未核实」的，开工前先复核，不要直接当结论。

### A. 数据保留与系统库卫生

| ID | 现状证据 | 要做 | 验收 |
| --- | --- | --- | --- |
| **A1** | 全仓无 `DELETE FROM sys_log_events` / `sys_metric_samples` / `sys_audit_events`；`internal/systemdb/logs.go` 只有 `GetRetention`/`PutRetention`（只存 `keep_days`，无执行者） | 加保留清理任务：按 `keep_days` 删过期行，挂到既有 maintenance（`systemdb/maintenance.go`）而不是新起 goroutine | 写入 3 天后触发一次，行数与 DuckLake 文件数下降；`__` 前缀内部表不受影响；清理失败不阻断启动 |
| **A2** | `sys_metric_rollups_hourly` 仅 `internal/systemdb/schema.go` 建表，零读写（v5.0 复核已判死表） | 二选一：①接线做小时聚合供长周期看板；②停止建表并从 `systemTables` 移除（走迁移，需审批） | 死表消失或出现唯一读写点；`SystemTableCount` 断言同步更新 |
| **A3** | v4.0 `console-polish` §1.6 九行验收指标（文件数 ≤5、登录 9 并发 <3s 冷 / <500ms 热、`logs` 冷 <1s 热 <200ms、`wait_total` 增速、`snapshot moved during copy` 归零）只在本地盘测过 | 在真实 COS 复测一次并记录数字 | 九行指标有实测值；未达标的项单独立项 |

### B. 读性能 / DuckLake

| ID | 现状证据 | 要做 | 验收 |
| --- | --- | --- | --- |
| **B1** | v4.0 `ducklake-rw-latency` P2-1…P2-4 全未做：用户库后台维护（flush+merge）、远端读缓存（`enable_external_file_cache`/`parquet_metadata_cache`/`http_keep_alive`）、`ListDocuments` 加 LIMIT/OFFSET 替代 `row_limit_exceeded`、COPY 前看 `WaitCount` 增量 | 按 P2 顺序逐项做，每项独立可回滚 | 300 次写后文件 ≤3；冷读降 ≥5×；列表接口支持分页且默认 50/上限 500 |
| **B2** | `internal/api/health.go` 的 `Ready` 每次直查 `checker.Ready`，无缓存（v3.0 api-db-perf P5 未做） | 加 1–2s 短 TTL 缓存，失败不缓存 | 探针 1s 一次时系统库查询次数下降；真实故障仍在 ≤2s 内翻红 |
| **B3** | 集合/建表路径每次 `CREATE TABLE IF NOT EXISTS`，无集合名缓存（v4.0 P2-3） | 加集合名→建表状态缓存（写路径失效） | 连续 100 次文档写不重复建表；集合 schema 变更后首次写生效 |
| **B4** | 首次打开 6.4s 来自 DuckDB `INSTALL` 扩展；生产是否预装**未核实** | 确认生产镜像预装扩展，缺则补 | 冷启 6.4s → 目标 <1s；启动日志无 `INSTALL` |
| **B5** | `godatabase/bench` 有读放大基准；`examples/` 与真实 COS 的 p95 **未验收** | 把 `bench` 结论与真实 COS 复测对齐成一张表 | 有「本地盘 / 假 S3 / 真实 COS」三列对比 |

### C. 多实例一致性（v3.0 DoD 全未勾）

| ID | 现状证据 | 要做 | 验收 |
| --- | --- | --- | --- |
| **C1** | 启动探针、`PutIfAbsent` 单测、`instance-identity.json` 变更拒绝：v3.0 §8 DoD 未勾；**未核实当前是否已补** | 先复核，再补启动探针与单测 | 两个实例同库启动时第二个明确拒绝（非静默双写） |
| **C2** | `pruneVersions` 失败只 `break`、`syncOnce` 忽略其返回值（v3.0 §3 静态确认）；**未核实** | 改为返回错误并上抛到同步结果 | 修剪失败不再被吞；`dirty==0` 时不走全量 COPY |
| **C3** | `local-state.json` 水位与 `syncOnce` 顺序修正（v3.0 §8）**未核实** | 复核后修 | 重启后水位不倒退；重复同步幂等 |
| **C4** | 系统库 `locator.json` 跨节点恢复未实现（v3.0 §3） | 设计并实现，或在 §5 明确降级为"单实例限制" | 文档、告警、`replicas=1` 三处口径一致 |

> C 组全部「未核实」——`godatabase` 是独立模块且近期有改动，开工前必须先读当前实现。禁止按 v3.0 原文直接改。

### D. 功能缺口

| ID | 现状证据 | 要做 | 验收 |
| --- | --- | --- | --- |
| **D1** | `examples/README.md` 已 2026-11-06 复核：G0/G3/G9/G14 关闭，仍适用 **G0b、G1、G2、G4、G6、G7、G8、G10、G11、G12、G13、G15、G16**（16 条平台缺口登记处） | 逐条决策：做 / 明确不做并写进 README | `examples/*/README.md` 的「限制」清单与实际能力一致；无悬空 G 编号 |
| **D2** | gofunction 版本 §14.4 七项全部未做：test 独立超时 `gofunction_invoke_timeout` + semaphore(2)、cron 写 `sys_go_func_invokes(channel=cron)`、复制响应按钮、「保存后设为生效」、历史版只读源码、`/go?version=` 开关、细分错误码（`version_not_found`/`invalid_exports`/`version_exists`，现统一映射） | 按 §14.4 顺序做，先错误码后超时与记账 | 七项各有用例；错误码不再统一映射 |
| **D3** | v2.0 `system-database-plan` Phase C/D：`llm` **stream 未落库**、`provider-catalog` 未做（`internal/api/router.go` 无该路由）、前端未接系统 API、系统库备份/只读开关、S3 索引 reconcile | 分两批：先 provider-catalog + stream 落库，再前端与备份 | 有 `provider-catalog` 路由与权限；stream 请求在 `sys_llm_*` 可查 |
| **D4** | 云沙盒 §11.3 真实 Cloud 生命周期与 `network=none` 冒烟**未执行**（缺 `SIMPLEBASE_SANDBOX_API_KEY`）；§15 后续四项（`cron_jobs.kind=sandbox_run`、自定义镜像注册、出网白名单、hostname-scoped secrets） | 有 key 时补冒烟；后续四项按需立项 | 冒烟脚本可重复执行；fake 结果不得等同真实 Cloud |
| **D5** | `docs/sdk/` 有 12 篇，但 `sandboxes.md`、`reference/errors.md`、`reference/config.md` **缺失**（v4.0 docs-site P2） | 补三篇 + 移动端/暗色/长文滚动人工验收 | 文档站无死链（`docs-content.test.ts`）+ 人工验收记录 |
| **D6** | `CatalogConfig.DatabaseID` 退役属 breaking（v2.0 cleanup L10），**是否有人设置 `SIMPLEBASE_CATALOG_DATABASE_ID` 未确认** | 确认后退役并写发布说明 | 环境变量无引用；发布说明含 breaking 段 |

### E. 前端

| ID | 现状证据 | 要做 | 验收 |
| --- | --- | --- | --- |
| **E1** | 覆盖率 functions 94.1% / branches 94.92%，低于 95% 阈值（`AGENTS.md` 现行基线，planv6.6 B2 后未变） | 补 `http-api-converters.ts` 与 `services/` 分支 | `yarn test` 三项均 ≥95% |
| **E2** | v2.0 `ui-polish-and-merge` 的 8 项里，**6 项已随重构消失**：任意字号只剩 `TraceIOBlock.vue` 2 处、`destructiveGhost` 已在使用、Monaco 已按主题切换、`SbModal` 已迁到 `components/modal/`。仅剩两套工具栏间距、表尾双 padding、Dashboard/CronJobs 双套状态文案 | 只做这三项 | 同一页面无两套间距；状态文案单源 |
| **E3** | `ui/src/pages/AgentManager.vue` 1110 行、`ui/src/services/http-api.ts` 987 行 | 拆 `AgentManager`（会话层/运行层/工具卡层）；`http-api.ts` 只留装配 | 单文件 <600 行；`yarn typecheck` 仍 0 错误 |

### F. 工程卫生

| ID | 现状证据 | 要做 |
| --- | --- | --- |
| **F1** | `internal/` 头注释仍含生成式「调用架构/主要符号」，已多处漂移（planv6.6 A1 经审批后选择保留） | 维持现状；新增文件不再写生成式头 |
| **F2** | `gofunction/bench_report.md` 指向已删目录 | 随 §2.4 一起改 |

### G. 评测与 Agent / CLI 能力（planv6.0–v6.1）

| ID | 现状证据 | 要做 | 验收 |
| --- | --- | --- | --- |
| **G1** | `gocloudagent/cli/config.go` 只有 `compact_*_tokens`，无输出上限项；上游 4096 是隐性兜底（v6.1 O5） | 加 `max_tokens` 配置 + CLI 参数 | 请求体带上限；超限走 `ErrMaxTokensExceeded` 且工具不执行 |
| **G2** | 无「无进展即停」判据；`supervisor P0②` 未做 | 定义 no-progress 判据（有写需求但无 diff 拒 stop 等） | 判据表 + 用例；误停率可评估 |
| **G3** | 云端仍是 `maxHistoryChars = 12000`（`cloudagent/prompt.go:15`、`history.go:76`）；CLI 已有 `compaction.go`，云端没有（v6.0 C3/E3） | 云端改 summarization + 按轮截断 | 长会话不再硬截断；token 预算可配 |
| **G4** | 全仓无 specialist 代码（仅 `gocloudagent/docs/eino-guide.md` 提及） | L2 specialists，默认关闭 | 开关 + 至少 1 个 specialist + 分流用例 |
| **G5** | `tool_execution_update` 全仓仅 docs/plan 提及 | 长工具期间推中间帧 | CLI 与沙盒都能看到增量进度 |
| **G6** | `gocloudagent/bench/run_smoke.py:354` 的 `verify_l3` 只验哨兵三态，没覆盖「命令发出去被挂载挡住」 | 补用例 | 命令真正执行后被 deny 时判 FAIL |
| **G7** | `run_smoke.py` 0 处解析 `usage`/`prompt_tokens`（代码侧 `end` 已带 token，harness 未接） | 报表打印 token 与费用；加「20 题费用超预算即停」护栏 | 报表含费用；护栏可触发 |
| **G8** | `.github/` 无 bench 相关 workflow | `workflow_dispatch` 评测入口 | 可手动触发并上传结果 |
| **G9** | `gocloudagent` 内 `supervisor|advisor` **0 命中** | P1–P5（Config/Plan/Accept、triggerMonitor、consult_supervisor、model→provider 路由、A/B 四臂） | 默认 `enabled=false`；开启后有 `supervisor_usage` 上限 |
| **G10** | `eval_test.go` 只存在于 `gofunction/internal/interp`；云端方案已被 `gocloudagent/bench/*.py` 顶替 | 正式废弃 E5 并写进文档，别留两套 | 文档与目录一致，无悬空引用 |

### H. 生产就绪（planv6.4 `production-readiness-plan`）

| ID | 现状证据 | 要做 | 验收 |
| --- | --- | --- | --- |
| **H1** | `GET /v1/system/readiness` **未挂载**：`/v1/system/*` 只有 `storage`；readiness 只在内建 `app/doctor.go` | 加 HTTP 端点（含各依赖 ready 与原因） | 返回结构化 readiness；无副作用；有 200/503 两态用例 |
| **H2** | `access_rule|owner_field|RowRule` **全仓 0 命中** | 行级 / owner 访问规则 | **上线前最大功能空洞**：只有登录没有授权。至少支持「按 owner 字段过滤读写」+ 越权 403 用例 |
| **H3** | `gofunction` 侧无 `functions.egress`；全仓无 `SIMPLEBASE_SECRETS_KEY` | 云函数出网控制 + 函数级 secrets | 可按函数配置白名单与 secret；secret 不进日志/审计 |
| **H4** | `internal/trigger/dispatcher.go:46` 仍是内存 `ErrQueueFull`；项目删除/导出/成员角色无 API | 触发器持久队列 + 项目生命周期 API | 队列落库可恢复；项目级 API 齐备 |
| **H5** | `examples/capabilities/` 止于 `15-llm` | 补 `16-production-profile` / `17-backup-restore` / `18-secrets` / `19-end-users` / `20-alerts` | 五条用例可跑通并进 `run_all.sh` |
| **H6** | 全仓无 `smtp|mailer` | 外部通知通道（SMTP/Webhook） | 告警可推送至少一种外部通道 |

### I. 平台能力缺口（planv6.2–v6.3）

| ID | 现状证据 | 要做 | 验收 |
| --- | --- | --- | --- |
| **I1** | PocketBase 对照里 6 项真缺：`relation`/`expand`（`internal/collection` 无 relation）、索引管理 API、集合 schema 导入+diff、集合重命名（只有 truncate）、每集合路径级限流、SMTP | 逐项先决定做/不做 | 每项有明确结论，不留悬空；12 项「明确不做」的理由保留在 §4.17 |
| **I2** | 沙盒 P1-1 违规监控（`Violation` 0 命中）、P1-2 祖先 pin、P1-3 `--args fd`、P2-1…P2-5 全未做 | 按编号实现；每项先写复现用例 | `Result.Violations` 存在并被 agent 消费；其余逐条 |
| **I3** | `gitdir:` 全仓 0 命中（`.git` 为文件时保护失效）；`run_linux_test.go` 7 条新用例与 `TestLinuxGitInitAndCommit` **未在 Linux 跑过** | 补 `gitdir:` 跟随 + CI Linux 任务 | CI 有真 bwrap 用例；worktree/submodule 下 `.git` 仍受保护 |
| **I4** | `docs/automation/sandbox-api.md` 仍写旧 `/v1/projects/:projectID/sandboxes`、`Idempotency-Key`；`sandbox.md` 仍写「没有 API Key」；`docs/` 全目录 `E2B` **0 命中** | 文档与 `mountE2BRoutes` 对齐；补 §4.6 差异表 + §4.9 取消清单 | 文档无旧路径；差异（本机 bwrap 非 Firecracker、无网络/模板/快照）写清 |
| **I5** | `app/cron_runner.go:41` `systemDBRunner` 与 `api/gofunction_handler.go` 的 `runSem`/`acquireRunSlot` 双分支仍在；`functionrun` 无 `RunFunction` | 云函数单一路径（N1） | cron 与 HTTP 走同一 Runner；删旧兜底 |

### J. examples 与本地开发（planv6.1 / v6.4）

| ID | 现状证据 | 要做 |
| --- | --- | --- |
| **J1** | `.simplebase/` 仍未进 `.gitignore`（E16 决策性搁置） | 加入忽略；否则对 `examples/<case>` 直接跑 skill 会留未跟踪目录 |
| **J2** | E7 的 5 把 `sb_live_` Key 指向已不存在的 `127.0.0.1:8080` | 环境事实无法从仓库修；文档注明「已失效，勿复用」 |
| **J3** | `local-dev P1` 四项无任何实施记录 | `/health/live` 带 version + apply 时 major 不符拒绝；漂移检测（远端多余函数/任务/表）；`pull`（云端导出成工作区）；CI 示例 |
| **J4** | `local-dev P2` 四项未启动 | 多环境 profile；种子审批；离线 tarball；控制台「从工作区导入」 |
| **J5** | `local-dev §8` 三条设计限制仍在 | `plan` 不能预演「建库之后」的步骤；引擎检查是项目级不能逐库；`expect.json` 严格比对需 Go/Python/fixture 三处同改 |
| **J6** | `references/` 只有 database/kv/s3/cron/logs/project/sandbox/site/gofunction/authoring，**无 user/apikey** | 认证进 skill；先定敏感写操作（建用户/签 Key）的确认流 |
| **J7** | `14-sandbox` 示例在本机 SKIP | 在具备 bwrap+Landlock 的 Linux 上复跑一次 |

### K. 工程债（planv6.0–v6.4）

| ID | 现状证据 | 要做 |
| --- | --- | --- |
| **K1** | `sys_databases.storage_prefix` 与 `catalog.Database.StoragePrefix` **仍未删**，`catalog/service.go:563` 仍用它作无 registry 时的删除退路（planv6.2 唯一架构偏离） | 删除退路改为按 `sys_projects.database_path`，再删列与字段 |
| **K2** | `Executor.Compile`、`Runtime.Cleanup` 仍是空实现，却写在 `gofunction/README.md` 的公开面 | 删掉或实现，并在 README 明确标注 |
| **K3** | `internal/api/perfbench` 仍在 `internal/api` 下（planv6.3 S1） | 挪出到独立包 |
| **K4** | `TestQuotaHandler/missing_project`、`TestAuditHandler/missing_project` 断言 500 但实际 400 | 断言改 `400 invalid_request` |
| **K5** | `gofunction_handler.go` 4 处 `system_project_protected` 错误构造未收敛（R2-A2） | 抽一处构造 |
| **K6** | 前端 typecheck 4 处既有测试类型错误（`.exists`、`page(pid,1)`）——**未核实是否已清零** | 复核；仍报错则修 |
| **K7** | `gosandbox/README.md:448` 未测「bwrap + helper」组合耗时；`:391` 未在真实委派环境读回 `memory.max` | 补两项实测，填入 README |
| **K8** | gofunction `ExecFib18` 实测 1.04ms，验收线 0.9ms（差 15%）；A4（`binop` 慢路径）未执行 | 要么实现 A4，要么把验收线改为 1.1ms 并注明归因（`callSSA`/`reflect.MakeFunc`） |
| **K9** | `gofunction/bench_report.md:106` 指向 `plan/planv6.5/gofunction-optimization-plan.md`——**该文件从未存在** | 改指本文 §3-K8 |

## 4. 仍然有效的事实规范（决策摘要 + 现行权威）

> 旧文档删掉后，这些决策只在这里有一句话；**详情一律以右列代码为准**，不要凭记忆改。

### 4.1 单写与持久化
单实例单写（`Config.Instance.Writable`），副本数=1、升级用 Recreate；S3 是唯一持久事实，**故障不得静默返回"已持久化"**；启动做 instance identity 冲突检测，但**不得宣称是分布式锁**。→ `internal/app/app.go`、`godatabase/lease`。

### 4.2 HTTP 契约
`{error:{code,message,request_id}}`；错误码以 `internal/api/error.go` 的 `sentinelErrorMappings`（表内顺序即优先级）为唯一来源；**SPA fallback 会把未匹配的 GET 返回 HTML**，手写 fetch 必须自行防御；`/go/...`、`/agent-threads/...` 等非 `/v1` 前缀路由注册在 fallback 之前。

### 4.3 DuckLake / 对象存储
三不变量：Parquet 以 UUID 命名、永不原地修改、catalog 决定可见性；提交协议三步，冲突重试不重写 Parquet。`durability ∈ {committed_local, synced_s3}`，`max_lag=30s` **不是**硬 RPO。平面 A（用户文件 `{env}/files/{projectID}/`）与平面 B（`{env}/ducklake/{dbID}/…`）物理隔离。参数：DuckDB ≥1.5.2、`max_retry_count=10` / `retry_wait_ms=100` / `backoff=1.5`、压缩分层 1→5→32MB、`debounce 200ms`、内联 1000 行且同步前 flush、三回滚开关（`data_inlining_row_limit:0` / `catalog_sync.mode:debounce` / `lease.enabled:true`）。

### 4.4 云 Agent 安全边界
Prompt 7 段组装顺序固定；**永不进 prompt**：S3 key、KMS/provider key、`credential_ref` 原文、DSN、`storage_prefix`，且只做一次 redact。只读工具 7 个 + 沙盒 4 个。SSE 帧：`run` / `token` / `thinking` / `tool_call` / `tool_progress` / `tool_result` / `error` / `end`；`tool_result` 后回到 `thinking`；`is_error` 不含沙盒非零退出。定时任务无请求上下文时构造内置 principal（只 `DatabaseRead`），CAS 认领 `WHERE id=? AND next_run_at=?`。

### 4.5 数据面契约
云函数签名 `func Name(req T) R`（恰好 1 入参 1 返回、`package main`）；cron `cron` vs `interval`（interval 以认领时刻推进防漂移）、tick 30s / 并发 4 / 执行超时 10s、error 300 字符 / response 4KB / input 8KB；集合名 `^[A-Za-z][A-Za-z0-9_]{0,62}$`；KV「缺失 key ≠ 404」（`GET`→null、`TTL`→-2/-1、`TYPE`→none、`DEL`→0、`SCAN`→`"0"`）；调用方已给 `databaseId` 时**禁止**静默落第一个库。

### 4.6 LLM / SDK
10 家预置厂商（openai/anthropic/azure_openai/google/deepseek/moonshot/glm/qwen/openrouter/custom_openai）的 id、协议、默认模型；`/agents/models` 只返名字，不返 `base_url`/`api_key`；LLM 未启用返 200 + 空。SDK：`SimpleBaseError{status, code, requestId, details}`；**浏览器暴露 Key = 该 Key 全部权限，当前无行级 RLS**；gofunction 基线见 `gofunction/bench_report.md`（fib(25) 259.8ms→52.4ms；`GOGC=off` 仅 -2.4% 说明瓶颈在分配路径）。

### 4.7 多实例风险（单写之外的禁区）
COS **不支持 `If-Match` on PUT**（AWS SDK 该字段被静默忽略），只用 `PutIfAbsent` 单原语 + 启动探针；快照 key `snapshots/{snap:020d}-{epoch:020d}`、`manifest/{seq:020d}.json`（`PutIfAbsent` 冲突即 split-brain 确证）；租约 `ttl 30s` / `renew 10s` / `grace 10s`、只用本地单调时钟；孤儿账本**只删"账本内 && 超期"**的文件；`delete_orphaned_files` 禁用于混用前缀。

### 4.8 前端
reka-ui 2.10.5 输出 `data-state`，**不输出裸 `data-checked`/`data-open`**；`TabsTrigger` 确有空的 `data-active`，新代码仍统一写 `data-[state=active]:`。i18n：`zh-CN.ts` 权威、`en-US.ts` 严格同构、组件外用 `i18n.global.t`、`@` 必须写成 `{'@'}`。加载态七状态判定表 + **160ms 出现延迟、消失不延迟**。对比度：`#d97757` 文字色仅 2.96:1，不可作正文色。

### 4.9 沙盒
错误码 10 条（`sandbox_unavailable` 503 / `not_found` 404 / `name_conflict` 409 / `limit_exceeded` 429 / `invalid_spec` 400 / `invalid_path` 400 / `file_too_large` 413 / `busy` 409 / `gone` 410 / `backend_error` 502，且不透传 SDK 路径）；非零退出=200 由 `exit_code` 表达、超时=200 + `timed_out` + `exit_code=-1`；默认 1C/256M、idle 5m、max_duration 30m、exec 30s（max 300s）、output 65536、file 1MiB、max_per_project 5、reap 5m；env 拒绝 `SIMPLEBASE_`/`MSB_`/`AWS_` 前缀；审计只记命令 SHA-256。

### 4.10 系统库性能与指标
日志选择性记录（只记写/错/慢）；summary/trend/quota 短 TTL 单飞缓存；`WriteGate` 按库 `TryDo`，`ErrAcquiring`→503+`Retry-After`。延迟直方图桶 `1,2,5,10,20,50,100,200,500,1000,2000,5000,10000,30000,+Inf`，**无样本返 `null` 而非 0**，`+Inf` 不可返回 Infinity/NaN，均值走 `SUM(http_latency_ms)/SUM(_count)`。

### 4.11 系统库只读与脱敏
敏感列 6 条：`password_hash`、`key_hash`、`refresh_token_hash`、`access_jti`、`credentials_json`、`credential_ref`（**不按子串猜**，`token_input`/`max_tokens` 保留）。两层：执行前标识符扫描 → 403 `system_column_redacted` 且不执行；执行后按结果列名抹 `null` + `redacted_columns`（`omitempty`）。`rows` 契约：`limit` 默认 50 最大 200、`offset` 最大 100000、404 `table_not_found`、`__` 前缀过滤。已知残留风险：`value_json`/`fields_json` 内密钥无法按列名识别。

### 4.12 CLI / 会话
CLI 信封 `{"ok":true,"data":…}` / `{"ok":false,"error":{code,message,http_status,request_id}}`，`code` 原样透传；退出码 0/1/2/3/4（含 `confirmation_required`、`config_insecure`）；配置 0600 + flag > env > file；委托 token 三条硬约束 + `project_mismatch`；前端 401 处理：匿名认证端点（`/v1/auth/login`、`/v1/auth/refresh`、`/v1/auth/password/initial`）不吃 `markUnauthorized`。

### 4.13 存储本地优先
`data_dir` 与 `cache_dir` **必须物理分开**：`cache.Manager.Evict` 按 LRU `removeAll` UUID 子目录，本地模式下本地盘是唯一副本，两者同路径会删掉真数据。`{data_dir}/_system/identity.json` 记录存储身份（local↔s3 不一致**拒绝启动**）。切换存储不迁移，直接 `./build.sh reset`（原计划的 `storage migrate` 已取消）。项目 ID 固定 8 位与 `_system/` 前缀不冲突。

### 4.14 云 Agent 评测基线（全仓唯一）
MBPP sanitized **348/427 = 81.5%**（13353s，prompt 2 083 058 / completion 397 903，¥2.78）；SWE-bench **Verified** 10 题 **5/10**（1843s，¥0.63，外推 500 题 ≈25.6h / ¥35）；Terminal-Bench **0/8**（4 题缺 `fmod@GLIBC_2.38`，1 题网关超时，`unsupported_libc` 不计分母）；冒烟 14/14。失败分类：断言 60 / 超时 15 / 运行错误 4。固定参数：`GOCLOUDAGENT_MAX_ITERATIONS=40`、自定义题 180s / HumanEval 120s / 评分 10s。**口径更正**：计划里的「SWE-bench Lite 20」实跑为「Verified 10/50」。三条已被代码证实的坑：`litellm.ProviderConfig.Timeout` 字段存在但从不被读、4096 是隐性兜底、默认值在 CLI 与云端分叉。

### 4.15 沙盒隔离边界
`gosandbox` **不得 import `internal/...`**；`gocloudagent` 只能 import `gosandbox/host`。隔离开启但不可用一律 fail-closed 返回 `isolation_unavailable`（不静默降级）。威胁模型：不承诺挡内核提权；工作区内误删挡不住。四条实现事实：Landlock 用严格 V3（非 V10+ 开关）、helper 失败信号 = 71 + stderr 前缀、cgroup 去开关恒降级并标「未限制」、网络只支持无网。性能基线 0.277ms（policy）/ 2.313ms（launch），**bwrap + helper 组合仍未测**。

### 4.16 godatabase 契约
`typed-nil Logger` 陷阱：`*zapLogger` 为 nil 赋给接口后**非 nil**，`if r.logger == nil` 失效 → 适配层必须传无类型 nil 或 `NopLogger`。别名必须是**同一个变量**（`var ErrNotFound = blob.ErrNotFound`）才能保证 `errors.Is` 恒等。依赖方向 `registry → filedb/ducklake/cache/lease → blob → 根包 → sqlx`，根包不 import 子包（否则成环）。`Spec` 只保留 6 字段，防业务字段倒灌。**D7 行为变化：系统库不再追加迁移，`EnsureSystemSchema` 对已存在表是 no-op，旧库必须 reset 才会补列。**

### 4.17 PocketBase 对照的「明确不做」
`§6` 12 项仍属不做：goja hooks、auth collection 全家桶、OAuth2 30+ 供应商、自更新、ACME、notify_watcher、迁移 down、impersonate、UI 扩展注入、安装向导、batch multipart、`VACUUM INTO` 单实现。D1–D5 决策全 A（例如集合级 API 规则改用 Scoped Key 而非 rules 表达式；缩略图由前端 Canvas 降级）。filter 语言子集是注入防线的事实来源：阈值 `max_filter_scan_rows=50000`、`in` 元素 ≤32、filter 长度 ≤2KB、展开上限 1024；cursor = `base64({c,i})` + `(created_at<?) OR (created_at=? AND id<?)`。

### 4.18 godatabase 性能基线
`TestReadLatencyPhases` 双档 p50/p95/p99；`SingleWriteCost` 2.75ms / 1393 newbytes（写放大归零）；KV `SET`/`GET`/`EXISTS`/`DEL` 分工况表，其中 `DEL 1000 key = 141.9ms` 是已知未优化项。批次 C 之前的基线未留存，P1–P6 净收益不可反推（方法学欠账）。R3「DuckDB 用户库快照可能不一致」**未复现**，已降为 P2 并只留回归用例。

## 5. 明确作废的旧结论（**不得再依据**）

| 旧结论 | 出处 | 现行事实 |
| --- | --- | --- |
| Turso/libSQL 作为数据库引擎、`internal/database/*` 路径 | v1.0 全部 | DuckDB/SQLite 单文件库 + DuckLake 系统库；`godatabase` 独立模块 |
| `internal/jobs`、`internal/deploy`、备份恢复 501 | v1.0 plan7/plan9、v2.0 databases-and-s3 | 三包已删；`BackupHandler.Restore` 已实现 |
| `/agent-schedules` 路由与 v26/v27 表 | v2.0 agent-schedule | 整体下线，统一 `/cron-jobs` |
| 用户库 open/close 状态机（`closing`/`closed`） | v1.0 §5.2、v3.0 database-always-open | 四态，不再暴露 open/close |
| antd / `tokens.css` / `--ant-*` / `max-width:1440px` | v1.0、v2.0 UI 系列 | reka-ui 2.10.5 + Tailwind；`ui-polish` 已明文否掉这三条 |
| 裸 `data-checked:` / `data-open:` 选择器可用 | v5.0 code-and-style §2.1 | reka 只输出 `data-state`；`data-active` 是唯一例外 |
| 「系统库恒非 SQL 数据模型」 | v5.0 cloud-assistant-sql-database-tabs §2#6 | 系统库现返回 `sql`，按 SQL 表展示 |
| KV「每项目一份 `kind=kv` 的 DuckLake catalog」 | v3.0 key-value-ducklake §2 | `godatabase/kv` + `__kv` 内部表，用户库内建 |
| 登录限速「同 IP+用户名 10 次/分 → 429」 | v3.0 login-auth §4.8 | 三维度软锁定（`internal/api/login_guard.go`）；**且"成功 IP 豁免"从未接线，已按产品决策删除描述与记账** |
| `principalMu` 全局锁导致认证 convoy | v5.0 db-read v2 §1B | 已是 per-key singleflight，锁内仅 map 操作无 I/O |
| typecheck 262 处错误 / KV `EXISTS` 待修 / `helpers.ts` Switch 假绿 | v5.0 code-and-style、§7.3 | 均已修（planv6.6 OPT-4；`helpers.ts` 用 `modelValue`） |
| `destructiveGhost` 零引用 / Monaco 未接暗色 / 任意字号泛滥 | v2.0 ui-polish §3 | 均已在用/已修，仅 `TraceIOBlock.vue` 残留 2 处任意字号 |
| `production-readiness-plan` 自述「规划（未实施）」 | v6.4 | **严重失真**：M1/M1.5/M4 已在代码（`config/profile.go`、`auth/keyring.go`、`sys_audit_events`、`alerts/*`、`backup/scheduler.go`）；按自述重做会重复实现 |
| `planv6.3/console-data-trigger-ops-fix` 自述「待实施」 | v6.3 | 已全部落地（JSON 导出、迟到响应丢弃、NavMenu 分组、触发器 Modal + `SbAsyncRegion`） |
| `planv6.0/openai-compatible` 自述「待确认后实施」 | v6.0 | 已上线并**超出计划**：多出 `/responses`；「`/chat/completions` 是唯一出口」已不成立 |
| `planv6.2`「本文是计划，不是已落地代码」、`module-refactor`「不代表代码已实施」 | v6.2 | 均已落地（M0–M7 / 独立模块 + `replace`） |
| `@技能` 用于选择模块 | v6.0 mentions §2 | **语义已反转**：skills 只由 `SkillsForModule(agent.Module)` 决定，`@` 只用于点名助手 |
| `microsandbox Cloud` 云沙盒方案 | v3.0 cloud-agent-sandbox | 从未实施，全仓 `microsandbox` **0 命中** |
| 独立 `scheduler.go` / `internal/agentjob` 包 | v6.0 阶段 A/B | 前者已删，后者改名 `internal/agentbridge`（`Logger:"agentjob"` 只是保留的日志检索口径） |
| `NewEvictor`、`OrphanLedger/MaintenanceGate` 属死代码（N9 的 B3/B4） | v6.3 | **已作废**：两者已在生产接线（`app.go:644`、`syncer_prune.go:63,104`），按旧文档删会删掉活代码 |
| 5 个存量文件 `gofmt` 未通过 | v6.2 utils §6 | 全仓已 clean（`gofmt -l internal cmd` 为空） |
| `internal/database/*`、`internal/utils/{sqlx,singleflight}` 路径 | v6.2 全部 | 已迁 `godatabase/{sqlx,internal/singleflight}`；`internal/utils/sqlval` 是 v6.3 后新增 |
| 「SWE-bench Lite 20 题」 | v6.1 benchmark §0 | 实跑为 **Verified 10/50**，引用时必须更正 |
| `E5 eval_test.go` 表驱动 8 案例 | v6.0 E5 | 已被 `gocloudagent/bench/*.py` 顶替，不要按原计划补 |
| `/agent-schedules` 兼容层 | v6.0 阶段 A5 | 已整体删除，仅测试里有字符串残留 |
| v6.0 chat-ux v1「阶段 A 逐页实测」 | v6.0 | 被代码审计替代，不必补测 |

## 6. 执行记录

删除范围与验证：

```bash
# 删除前（内容已全部落到本文件；恢复：git checkout <sha> -- plan/planv1.0 …）
git rev-parse --short HEAD          # 99ab993
rm -rf plan/planv1.0 plan/planv2.0 plan/planv3.0 plan/planv4.0 plan/planv5.0
rm -rf plan/planv6.0 plan/planv6.1 plan/planv6.2 plan/planv6.3 plan/planv6.4

# 删后复核：全仓不得再有指向这十个目录的路径引用
rg -n 'plan/planv([1-5]|6\.[0-4])\.' --glob '!plan/planv6.7/**' . || echo OK
```

| 项 | 结果 |
| --- | --- |
| 第一轮删除 | `planv1.0`(14) / `planv2.0`(27) / `planv3.0`(11) / `planv4.0`(11) / `planv5.0`(10) = **73 个文件** |
| 第二轮删除 | `planv6.0`(11) / `planv6.1`(7) / `planv6.2`(7) / `planv6.3`(5) / `planv6.4`(6) = **36 个文件** |
| 保留目录 | `planv6.5` / `planv6.6` / `planv6.7`（未动） |
| 搬运落点 | 待办 → §3（11 组 60 条）；契约 → §4（18 组）；锚点 → §2（含 §2.5 的 v6.0–v6.4 编号） |
| 外部引用改动 | 第一轮 **19 个文件**（见下）；第二轮新增 **12 个文件**：`AGENTS.md`（计划目录段重写）、`examples/README.md`、`docs/getting-started/openai-compatible.md`、`docs/workbench/agent-e2e.md`、`gocloudagent/bench/README.md`、`gofunction/bench_report.md`（2 处，含修掉指向 `planv6.5/gofunction-optimization-plan.md` 的**历史悬空引用**）、`config.example.yaml`、`internal/api/storage_settings_handler.go`、`ui/src/composables/useLoadState.ts`、`ui/src/constants/agent-prompts.ts` |
| 删后复核 | `rg 'plan/planv([1-5]\|6\.[0-4])\.' --glob '!plan/planv6.7/**' .` → **0 命中**；`go build ./internal/... ./cmd/...` 通过 |

> 未做的事：两轮都不动源码；§3 的 60 条一条都没开工。C 组（多实例）开工前必须先复核 `godatabase` 当前实现——那几条是「未核实」而非「确认未做」。
