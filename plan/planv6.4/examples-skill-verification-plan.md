# examples 体检与 simplebase skill 逐站验证（planv6.4）

> **状态：** 已执行（2026-11-06；P1–P6 完成，见 [`examples-skill-verification-report.md`](./examples-skill-verification-report.md)）
> **日期：** 2026-11-06
> **关联：** `examples/`、`skills/simplebase/`（`SKILL.md`、`scripts/sbdev.py`）、`internal/sbcli/dev*.go`、`plan/planv6.4/local-dev-cloud-replay-collab-plan.md`（P0a/P0b）、`plan/planv4.0/examples-business-cases-plan.md` §6（G0b–G16）

> **执行结论（摘要）：** E1–E6 全部修复并验收；4 个站点在 A/B 双实例下 S0–S10 **0 FAIL**（shop/iot PASS，ops/community PARTIAL，唯一原因是平台缺陷 BUG-1）；`run_all` 扩到 15 个用例并全量回归通过（默认 131 步 / 开备份 144 步，exit 0）。执行中新发现 2 个 P0 样例问题（单项目单库导致 community 双库失效、init.sh 的 Key 轮换/agent-schedule/MIME 三处失败）与 2 个平台缺陷（BUG-1 集合种子不可幂等、BUG-2 删库清空项目单文件），已在报告 §3 记录；原计划中「B 无 --yes 退出 3」「community 双库」等断言已被实测修正，详见报告 §3.5 与本文件 §9。


**Goal：**

1. 判断 `examples/` 现有样例是否有问题，是否覆盖 SimpleBase 的主要能力场景。
2. 用 `skills/simplebase`（`sbdev.py` 与 `simplebase dev`）把每个业务站点（shop / community / iot-telemetry / ops-assistant）从零跑起来，确认「工作区 → 本地 → 另一实例（模拟云端）」只换 URL + Key 就能用。

**结论先行：**

- 样例**能跑的前提很窄**：只在 Linux、8080 空闲、用 `examples/init.sh` 时成立。macOS 下能力示例全部失败；有 1 个站点（iot-telemetry）不能直接被 skill 校验通过；云助手的 cron 用例必然失败。
- 覆盖面：数据（SQL/文档/KV/对象）、函数、cron、Key 权限覆盖较好；**实时订阅、函数触发器、备份恢复、沙箱、集合 schema/导入导出、SQL 迁移接口、OpenAI 兼容接口、助手定时**没有可运行样例。
- skill 侧：四个站点里三个能直接 `validate` 通过；但清单不写 `data_model` 时 skill 与 `init.sh` 推断结果不同，同一实例先后用两种入口会触发 `data_model_mismatch`；KV / 对象 / Key / `trigger_once` 不在重放范围，站点「跑起来」还缺一段补位脚本。

---

## 1. 已做的现状核验（2026-11-06，本机 macOS）

| # | 检查 | 命令 | 结果 |
| --- | --- | --- | --- |
| 1 | init 清单 dry-run | `./examples/init.sh --dry-run` | 通过（4 个项目、DDL 检查无违规） |
| 2 | Go 样例静态检查 | `go vet ./examples/...` | 通过 |
| 3 | JS / Bash 语法 | `node --check`、`bash -n` | 通过 |
| 4 | skill 单测 | `python3 -m unittest discover -s scripts -p 'test_sbdev.py'` | 28/28 通过 |
| 5 | 二进制离线校验 | `./simplebase dev validate --dir examples/<case>` | shop / community / ops-assistant 通过；**iot-telemetry 失败**：`cron_jobs[1].run_at 必须是 RFC3339 时间` |
| 6 | 脚本离线校验 | `sbdev.py validate -C examples/<case>` | 与 5 一致（两端对账正常） |
| 7 | 毫秒计时 | `date +%s%3N` | macOS 输出 `17939516983N`，`$((end - start))` 报 `value too great for base` |
| 8 | 端口 | `lsof -iTCP:8080` | 本机 8080 被 nginx 占用，`/health/live` 返回 nginx 404 |

> 第 5/6 项的 4 条 warning（忽略 `api_keys`/`kv`/`objects`/`trigger_once`）符合设计，不算问题。

---

## 2. 样例问题清单

按严重度排序。P0 = 阻断运行；P1 = 结果错误或误导；P2 = 卫生 / 体验。

### 2.1 P0：阻断

| ID | 位置 | 问题 | 影响 | 修复 |
| --- | --- | --- | --- | --- |
| E1 | `capabilities/lib/common.sh` `sb_raw`、`run_agent.sh` | `date +%s%3N` 是 GNU 扩展；BSD date 输出 `...3N`，算术展开报错，`set -e` 下脚本直接退出 | macOS 上 01–08 每个用例在第一步就失败 | 抽 `now_ms()`：优先 `python3 -c 'import time;print(int(time.time()*1000))'`，或 `perl -MTime::HiRes` |
| E2 | `run_all.sh:33`、`run_agent.sh:723` | `PATH` 硬编码 `/home/ubuntu/.nvm/versions/node/v22.23.3/bin` | 只在作者的机器上有意义；其他机器无害但误导 | 删除；改为 `command -v node` 检查并给出提示 |
| E3 | `iot-telemetry/simplebase.json` `iot_export_once` | `schedule_kind: once` 没写 `run_at`，靠 `run.mjs` 运行时补 | skill 与 `simplebase dev validate` 直接拒绝，**这个站点无法用 skill 跑起来** | 清单写固定远期 `run_at`（如 `2099-01-01T00:00:00Z`），`trigger_once` 照旧由 init 触发；`run.mjs` 中补 `run_at` 的分支删掉 |
| E4 | `capabilities/agent/cases.json` `cron` 用例 | 要求 `func-file atick{{RUN_ID}}`，但没有任何步骤创建这个函数；服务端 `validateTarget` 会返回 `gofunction_not_found` | 云助手 cron 用例必然失败（或模型自行补建函数，结果不可预期） | 用例前置：`run.mjs` 在该用例前用 CLI 建 `atick<RUN_ID>`（复用 `08-cron/tick.go`），或把 prompt 改为先建函数 |
| E5 | 数据目录与端口 | `run_all.sh` / `init.sh` 默认 `http://127.0.0.1:8080`；8080 被占用时 `health` 失败，提示却是「use --start」 | 本机无法启动 | `run_all.sh --start` 支持 `--api-port`，透传给 `./build.sh dev --api-port`；health 失败时打印实际响应码 |

### 2.2 P1：结果不对或误导

| ID | 位置 | 问题 | 修复 |
| --- | --- | --- | --- |
| E6 | 4 个 `simplebase.json` | 不写 `data_model`。skill 推断：shop / iot-telemetry 为 `sql`（有迁移、无集合）；`run.mjs` 强制 `collection`。同一实例先 init 再用 skill（或反之）报 `data_model_mismatch` | 清单显式写 `"data_model": "collection"`（与已有环境一致）；删掉 `run.mjs` 里的强制覆盖。新项目若要 `sql`，在 README 写明需新项目 ID |
| E7 | `examples/*-service/`（booking / community / mini-game / shop / ticket） | 未跟踪的遗留目录，只剩 `.env`（含**有效格式的 `sb_live_` Key**，项目 `6n79jc0h`）和 `.DS_Store` | 吊销对应 Key 后删除目录；`examples/README.md` 不再出现这些名字 |
| E8 | `community/.env` | 双库站点只写了第一个库的 `SIMPLEBASE_DATABASE_ID`；`media` 库 ID 无处可取 | `keys()` 同时写 `SIMPLEBASE_DATABASE_ID_<NAME>`，与 `dev apply` 生成的 `.simplebase/env.<label>` 一致 |
| E9 | `run.mjs` `resetSeeds` | 只读 `001_init.sql` 推导要清空的表；后续迁移新增的表不会被清 | 遍历迁移目录所有 `*.sql`，或改为按 `schema show` 列表清空（排除 `_migrations`） |
| E10 | `lib/goexample/cmd/build` | 复制 `web/{index.html,style.css,app.js}`，但没有任何站点有这三个文件，也没有调用方 | 删除 |
| E11 | `capabilities/README.md` 与 `agent/README.md` | 写「`kv`/`gofunction`/`cron` 助手需要服务端启用 CLI 技能」，但 `run_all.sh` 没有检查开关，失败信息只在 jsonl 里 | `run_agent.sh` 先 `GET /agents` 判断，缺失时整体标 `skip` 并打印原因 |
| E12 | `shop/README.md` | 「实时云函数」容易被理解为 realtime 订阅；实际是 BFF 同步调用 `pricing.Quote` | 改为「下单时同步调用云函数算价」 |

### 2.3 P2：卫生

| ID | 问题 | 处理 |
| --- | --- | --- |
| E13 | `01-project` 每次建 `cap-<RUN_ID>` 项目、`06`/`08` 建的函数不删；CLI 没有 `project delete` | 用例末尾清理函数；项目复用 `PROJECT_ID_FILE`（存在就跳过 create，加 `--fresh` 才新建） |
| E14 | 业务站点没有自动化冒烟：BFF 路由只写在 README | 见 §4 Phase 4，加 `examples/<case>/scripts/smoke.sh` |
| E15 | `examples/README.md` 指向 `planv4.0` 的 G0b–G16，部分缺口已被后续版本修复（如 M1 跨项目 403） | 本计划执行时逐条复核，已修复的从 README 删去 |
| E16 | `.simplebase/` 不在仓库 `.gitignore`；直接对 `examples/<case>` 跑 skill 会写出 `lock.json` 与 `env.*` | 验证在临时副本里做（§4）；是否把 `examples/*/.simplebase/` 加进 gitignore 由维护者决定（本计划不改配置文件） |

---

## 3. 能力覆盖矩阵

来源：`internal/sbcli/commands.go` 的命令表、`internal/api/*_handler.go`、`docs/`。✅ 有可运行样例且有断言；◐ 只在业务站点里间接用到、无断言；❌ 无样例。

| 能力域 | 子能力 | capabilities | 业务站点 | 云助手 | 判定 |
| --- | --- | --- | --- | --- | --- |
| 项目 | 列表 / 创建 / 设置 / 配额 / 审计 / 日志检索 / 日志保留 | 01 | init.sh 建项目 | — | ✅（缺：日志保留 `set`、指定 ID 建项目） |
| 用户与鉴权 | 用户增删查、只读 Key、写拒绝、吊销 | 02 | ops `verify-perms.sh`、init 跨项目 403 | readonly-guard | ✅（缺：`user update/disable`、刷新令牌） |
| SQL 库 | 建库、建表、加列、CRUD、`schema rows` | 03 | 4 站点 | sql-crud | ✅ |
| SQL 事务 | `sql batch --transactional` | ❌ | shop 下单、iot worker | — | ◐ |
| SQL 迁移接口 | `migrate plan/status` | ❌ | ❌ | — | ❌ |
| 文档 | 集合库、增删改查 | 04 | community、ops | documents-delete | ✅ |
| 文档进阶 | `document patch/batch/import/export`、`collection schema-get/put/truncate`、过滤/投影 | ❌ | ❌ | — | ❌ |
| KV | String / Hash / INCR / DEL | 05 | 4 站点种子 | kv-delete | ✅ |
| KV 进阶 | TTL / List / Set / ZSet / SCAN | ❌ | 种子与 BFF 用到 | — | ◐ |
| 云函数 | 创建、调用、版本、试跑、激活 | 06 | 4 站点 + BFF `/go/...` | function-invoke | ✅（缺：`function list/delete/update`、`/go` 直调鉴权） |
| 定时任务 | interval 创建、触发、运行记录、删除 | 08 | 4 站点（cron / interval / once） | cron（E4 失败） | ✅（缺：`cron update`、`enabled` 切换） |
| 对象存储 | 上传、列举、预签名、删除 | 07 | shop 图片、community 附件、iot 导出 | object-delete | ✅ |
| 函数触发器 | 记录 / 对象事件触发、HTTP invoke | ❌ | ❌ | — | ❌ |
| 实时订阅 | SSE `/realtime`、订阅管理 | ❌ | ❌ | — | ❌ |
| 备份恢复 | `backup create/list/restore/delete` | ❌ | ❌ | — | ❌ |
| 沙箱 | `sandbox create/connect/pause/delete`、E2B 兼容 | ❌ | ❌ | — | ❌ |
| LLM | OpenAI 兼容 `/v1/chat/completions`、`/v1/models` | ❌ | community summary（无 provider 时 501） | — | ◐ |
| 云助手 | SSE 运行、确认、只读拦截 | — | ops `agents` 种子 | agent/* | ✅（缺：助手定时 `agent-schedules`） |
| SDK | JS SDK | — | shop、iot | — | ◐ |
| SDK | Go SDK | — | community、ops | — | ◐ |
| 本地→云重放 | `dev validate/plan/apply/status`、`sbdev.py` | ❌ | init.sh 内部调用 | — | ◐（无「两个实例」的端到端样例） |

**缺口优先级**（按「用户最常问 + 平台差异化」排序）：

1. **本地→云重放端到端**（本计划 Phase 2–5 直接补上）。
2. 函数触发器、实时订阅：BaaS 核心卖点，目前零样例。
3. 备份恢复：运维必需。
4. 文档进阶（patch / import / export / schema）。
5. 沙箱、LLM：依赖外部条件（容器、provider），做成可选用例，条件不满足时 `skip`。

---

## 4. 逐站验证方案（基于 `skills/simplebase`）

### 4.1 原则

- **按 skill 的工作流走**，不用 `examples/init.sh` 代替：`init`/`doctor` → `validate` → `plan` → `apply` → CLI 验证 → 远端 `plan` → `apply --yes` → 换 URL + Key 跑业务。目的是验证「真实用户拿 skill 能不能把站点跑起来」。
- **两套实例模拟本地 / 云端**：同一二进制，不同端口、不同 `SIMPLEBASE_DATA_DIR`。A = 本地（`:18080`，`data-a`），B = 云端（`:18081`，`data-b`）。避开被占用的 8080，也不碰仓库 `./data`。
- **工作区用临时副本**：`cp -R examples/<case> $WORK/<case>`，skill 产生的 `.simplebase/`、`.env*` 只落在副本里，不污染仓库（对应 E16）。
- **Key 不进对话和日志**：项目创建、Key 签发由 harness 用超管 JWT 完成，写入副本的 `.env` / `.env.cloud`（0600）；输出统一过 `common.sh` 的 `redact`。
- skill 不负责的部分（建项目、签 Key、KV / 对象种子、`trigger_once`、agents）由一个**补位脚本**完成，和 skill 的边界一一对应，不混入重放。

### 4.2 目录与产物

```text
scripts/examples-verify/            # 新增（Phase 1 实现）
  verify.sh                         # 入口：起 A/B 实例 → 逐站跑 → 汇总
  lib.sh                            # now_ms、redact、record（从 capabilities/lib/common.sh 抽）
  bootstrap.mjs                     # 补位：建项目(同 ID)、签 Key、写 .env*/.env.cloud、kv/objects 种子、trigger_once
  smoke/<case>.sh                   # 每站业务冒烟（Phase 4）
/tmp/sb-verify/<RUN_ID>/
  data-a/ data-b/                   # 两个实例的数据目录
  work/<case>/                      # 工作区副本
  results.jsonl                     # 每步一行：{suite:"skill", case, env:"A|B", step, ok, ms, detail}
  report.md                         # 汇总表（Phase 5 输出）
```

### 4.3 每个站点的步骤（S0–S9）

对 `shop`、`community`、`iot-telemetry`、`ops-assistant` 各执行一遍。`$SK = skills/simplebase/scripts/sbdev.py`，`$W = work/<case>`。

| 步骤 | 动作 | 命令 | 通过条件 |
| --- | --- | --- | --- |
| S0 | 准备副本 | `cp -R examples/<case> $W` | — |
| S1 | 离线校验（两端对账） | `python3 $SK validate -C $W`；`simplebase dev validate --dir $W` | 两者都退出 0，warning 集合一致 |
| S2 | 建项目 + 签 Key（补位） | `node bootstrap.mjs --url A --project <id> --keys publish,service` | A 上有项目 `<id>`；`$W/.env` 有发布 Key（`database:admin`），`$W/.env.runtime` 有运行时 Key（read/write） |
| S3 | 自检 | `python3 $SK doctor -C $W` | 全部 `[ok]` |
| S4 | 本地 plan | `python3 $SK plan -C $W --seed` | 动作全为 `create`/`apply`；保存输出 `plan-A.txt` |
| S5 | 本地 apply | `python3 $SK apply -C $W --seed` | 退出 0；动作序列与 S4 相同（`TestDevPlanMatchesApplyActions` 的真实环境版） |
| S6 | 幂等 | 再次 `apply --seed`；再用 `simplebase dev apply --dir $W --seed --yes` 跑一次 | 两次全部 `skip`；**两种入口交替使用不报 `data_model_mismatch`**（检验 E6） |
| S7 | 资源核验 | CLI：`database list`、`sql query`（表行数）、`collection list`、`function invoke`（用清单 `test`）、`cron list` | 见 §4.4 每站断言 |
| S8 | 补位 + 业务冒烟（A） | `bootstrap.mjs --seed-data`（kv/objects/trigger_once）；起 BFF；`smoke/<case>.sh` | 见 §4.4 |
| S9 | 云端重放（B） | B 上用**同一 project ID** 建项目签 Key → `.env.cloud`；`plan --env-file .env.cloud`（不带 `--yes` 的 `apply` 退出码 3）；`apply --env-file .env.cloud --yes --seed`；再跑一次全 `skip` | 退出码 3 / 0 / 0 符合预期 |
| S10 | 只换 URL + Key | 把 BFF 的 `SIMPLEBASE_URL`、`SIMPLEBASE_API_KEY` 换成 B，`SIMPLEBASE_DATABASE_ID` 从 `$W/.simplebase/env.<label>` 取；重跑 `smoke/<case>.sh` | 与 S8 结果一致；**不改任何业务代码** |
| S11 | 变更回放 | 在副本新增 `002_verify.sql`（加一列）、改函数源码（返回值加字段）；A 上 apply → B 上 plan/apply | 只执行 002；函数新版本试跑后才生效；B 的 plan 显示 `update function` |
| S12 | 负向用例 | 改已应用的 `001_init.sql` → apply；在函数里写假 `sb_live_` → validate | 前者 `migration_modified`（退出 1）；后者 validate 失败 |

S11、S12 每站都跑代价高，只在 `shop`（单库）和 `community`（双库 + 集合）上跑。

### 4.4 每站断言

| 站点 | S7 资源断言 | S8/S10 业务冒烟 |
| --- | --- | --- |
| shop | `products` 20 行；`orders`/`order_items`/`daily_stats` 存在；`pricing.Quote({"items":[{"price_minor":100,"qty":2}],"coupon":"DEMO10"})` → `total_minor=180` | `GET /api/products` 20 条且带 `image_url`；同一 `Idempotency-Key` 下单两次只出一单；库存扣减 1 |
| community | 两库 `community`、`media`；`posts` 集合含 `demo`；`likes` 1 行；`moderation.Check` 对 `spam` 返回 `allowed=false` | `POST /api/posts` 成功；含 spam 被拒；重复点赞只计一次；`DELETE` 后附件对象一并删除；无 provider 时 summary 返回 501 |
| iot-telemetry | `devices` 2 行；`rollup.Window` 返回 `start`/`end`；`iot_rollup`（interval 300）与 `iot_export_once`（once）都存在 | `scripts/curl.sh` 两次请求 200；`simulate.mjs` 写入 1 行；`worker.mjs` 连续跑两次，第二次不重复处理同一 run |
| ops-assistant | `tickets` 1 行；`ticket_meta` 集合含 `ticket-demo`；`sla.Evaluate` 对过期工单返回 `overdue` | `GET /api/tickets`、`/api/health` 200；`verify-perms.sh` 全 PASS（跨项目用例的目标改为参数，不写死 `ex-shop1`） |

### 4.5 执行方式

```bash
# 0. 构建（一次）
./build.sh                      # 产出根目录 simplebase；JS SDK dist 已存在则跳过

# 1. 全量（起 A/B → 4 站 × S0–S10 → 汇总）
scripts/examples-verify/verify.sh

# 2. 常用参数
scripts/examples-verify/verify.sh --only shop,community   # 指定站点
scripts/examples-verify/verify.sh --skip-cloud            # 只跑 A（S0–S8）
scripts/examples-verify/verify.sh --with-changes          # 追加 S11/S12
scripts/examples-verify/verify.sh --keep                  # 保留 /tmp/sb-verify/<RUN_ID> 与实例进程，便于排查
scripts/examples-verify/verify.sh --ports 18080,18081

# 3. 单步手工复现（与 skill 文档一致）
python3 skills/simplebase/scripts/sbdev.py plan  -C /tmp/sb-verify/<RUN_ID>/work/shop --env-file .env.cloud
python3 skills/simplebase/scripts/sbdev.py apply -C /tmp/sb-verify/<RUN_ID>/work/shop --env-file .env.cloud --yes --seed
```

`verify.sh` 的实例启动方式：

```bash
SIMPLEBASE_HTTP_ADDRESS=127.0.0.1:18080 SIMPLEBASE_DATA_DIR=$RUN/data-a SIMPLEBASE_DB_CACHE_DIR=$RUN/cache-a ./simplebase >$RUN/a.log 2>&1 &
SIMPLEBASE_HTTP_ADDRESS=127.0.0.1:18081 SIMPLEBASE_DATA_DIR=$RUN/data-b SIMPLEBASE_DB_CACHE_DIR=$RUN/cache-b ./simplebase >$RUN/b.log 2>&1 &
# 等 /health/ready == 200（最多 60s）；超管用种子账号登录；退出时 trap 结束两个进程
```

> 两个实例都用 local 存储；`database.cache_dir`（`SIMPLEBASE_DB_CACHE_DIR`）必须分开且不能包含 `data_dir`，否则缓存淘汰会互删。

### 4.6 判定与报告

- 每站给出三态：**PASS**（S1–S10 全过）、**PARTIAL**（重放过，业务冒烟有已知缺口，附 G 编号）、**FAIL**。
- `report.md` 一张总表：站点 × 步骤，失败格写 `error.code` 与一行原因；末尾列「skill 文档与实际行为不一致」的条目，回灌到 `SKILL.md` / `references/*`。
- 退出码：任一站 FAIL → 1；只有 PARTIAL → 0 并打印警告。

---

## 5. 分阶段执行计划

| Phase | 内容 | 主要改动 | 完成标准 |
| --- | --- | --- | --- |
| **P1 修阻断** | E1–E6 | `capabilities/lib/common.sh`、`run_agent.sh`、`run_all.sh`（加 `--api-port`）、`iot-telemetry/simplebase.json`（写死 `run_at`）、4 个清单显式 `data_model`、`agent/run.mjs`（cron 用例前置建函数）、`lib/init/run.mjs`（删 once/data_model 覆盖） | macOS 上 `run_all.sh --start --api-port 18080 --skip-agent` 全绿；4 个站点 `simplebase dev validate` 全部退出 0 |
| **P2 验证 harness** | §4.2 的 `verify.sh`、`lib.sh`、`bootstrap.mjs` | 新增 `scripts/examples-verify/` | `verify.sh --only shop --skip-cloud` 跑通 S0–S8 |
| **P3 云端重放** | S9–S10 | `verify.sh` 起 B 实例；`bootstrap.mjs --target B` | 4 站 S9 退出码符合预期；S10 只换 env 冒烟通过 |
| **P4 业务冒烟** | §4.4 的 `smoke/<case>.sh`；`verify-perms.sh` 去掉写死的 `ex-shop1` | 新增 4 个冒烟脚本 | 4 站 PASS 或 PARTIAL（附缺口编号） |
| **P5 清理与卫生** | E7–E16 | 吊销并删除 `*-service/`；`keys()` 多库 env；`resetSeeds` 扫全部迁移；删 `goexample/cmd/build`；README 更新 | `git status examples` 只有预期改动；README 与实际行为一致 |
| **P6 补覆盖** | §3 缺口 1–5 | 新增 `capabilities/09-triggers`、`10-realtime`、`11-backup`、`12-documents-advanced`、`13-dev-replay`；可选 `14-sandbox`、`15-llm`（条件不满足时 skip） | 覆盖矩阵中 ❌ 只剩明确标为可选的项 |

建议顺序：P1 → P2 → P3 → P4 并行 P5 → P6。P1–P4 是本次诉求的主体；P6 视排期。

### P6 新增能力样例要点

| 目录 | 步骤 | 断言 |
| --- | --- | --- |
| `09-triggers` | 建函数 → `POST /function-triggers`（集合 create 事件）→ 插文档 → 查触发记录；HTTP invoke | 函数被调用一次；队列满返回 429 不在此测 |
| `10-realtime` | 后台 `curl -N .../realtime?collections=notes&events=create` → 插文档 | 5 秒内收到含该 id 的事件 |
| `11-backup` | 建库写数据 → `backup create` → 删行 → `backup restore` → 查询 | 数据恢复；`backup delete` 成功 |
| `12-documents-advanced` | `document patch`、`batch`、`export` → `import` 到新集合、`collection schema-put` 后插非法文档 | 导入行数一致；schema 校验拒绝非法文档 |
| `13-dev-replay` | 内置最小工作区（1 迁移 + 1 函数 + 1 cron），`dev plan/apply` 两次 | 第二次全 skip；与 §4 共享断言库 |

---

## 6. 优化建议（超出修复本身）

1. **清单补齐 skill 不覆盖的声明**：`kv`、`objects` 已在 P1 计划纳入 `--seed`（local-dev-cloud-replay §5 P1）。落地后 `bootstrap.mjs` 的种子部分可删，`init.sh` 只剩建项目 + 签 Key，站点完全由 skill 驱动。
2. **CLI 支持指定项目 ID**：`project create` 只有 `--name/--engine`，而 skill 要求两端同 ID，只能走控制台或裸 HTTP。加 `--id` 后 `bootstrap.mjs` 可改用 CLI，skill 文档里的「去控制台建项目」也可给出命令行替代。
3. **合并两套初始化**：`examples/init.sh` 与 `verify.sh` 的补位逻辑重复。P5 后让 `init.sh` 复用 `bootstrap.mjs`，只保留一份实现。
4. **CI 接入**：GitHub Actions（Linux）跑 `verify.sh --skip-agent`；macOS runner 每周一次，专门防 E1 这类平台差异回归。`dev validate` 四个站点放进 `scripts/test.sh` 作为快速门禁。
5. **结果格式统一**：`capabilities`、`agent`、`skill` 三个 suite 都写同一 jsonl schema（已有 `suite/case/step/ok/ms/detail`，加 `env` 字段），汇总脚本只有一份。
6. **样例 README 模板化**：每站固定「运行 / 验证清单 / 已知缺口 / 用 skill 重放」四节，最后一节直接给 S1–S10 的命令。

---

## 7. 风险与应对

| 风险 | 应对 |
| --- | --- |
| 两个实例共享本机端口或缓存目录 | 端口参数化；启动前检测端口；数据 / 缓存目录全部落在 `/tmp/sb-verify/<RUN_ID>` |
| 显式 `data_model: collection` 后，已经用 skill 建出 `sql` 库的环境报 mismatch | 只影响按旧 skill 推断建过库的环境；README 写明用新项目 ID 或删库重建 |
| `trigger_once` 在 B 上重复触发造成副作用 | 补位脚本沿用「首次创建且无运行记录才触发」的规则；S9 不触发，只在 S10 前由补位脚本执行一次 |
| LLM / 沙箱 / CLI 技能未启用导致大量失败 | 统一判定为 `skip` 并写原因，不计入 FAIL |
| 遗留 `*-service/.env` 里的 Key 仍有效 | P5 第一步先吊销（项目 `6n79jc0h`，需要该实例超管），再删文件 |

---

## 8. 验收

1. macOS 与 Linux 上 `./examples/run_all.sh --start --skip-agent` 全绿（P1）。
2. 4 个站点 `simplebase dev validate` 与 `sbdev.py validate` 都退出 0，warning 一致（P1）。
3. `scripts/examples-verify/verify.sh` 4 站全部 PASS 或 PARTIAL，`report.md` 给出每站 S1–S10 结果（P2–P4）。
4. S6：同一实例上 `sbdev.py apply` 与 `simplebase dev apply` 交替执行，全部 `skip`，无 `data_model_mismatch`。
5. S10：4 个站点的 BFF / 脚本只改 `SIMPLEBASE_URL`、`SIMPLEBASE_API_KEY`（及按库名取的库 ID）即可在 B 上跑通。
6. `examples/` 下不再有未跟踪的 `.env` Key 文件（P5）。
7. 覆盖矩阵中 ❌ 项只剩标注为可选的沙箱 / LLM（P6）。

---

## 9. 执行记录（2026-11-06）

> 完整结果、证据路径与建议见 [`examples-skill-verification-report.md`](./examples-skill-verification-report.md)。

### 9.1 阶段完成情况

| 阶段 | 结果 |
|---|---|
| P1（E1–E6） | ✅ 4 站双入口 `validate` 全 0；`run_all --skip-agent` exit 0（63/63） |
| P2（harness） | ✅ 新增 `scripts/examples-verify/`（verify.sh / lib.sh / bootstrap.mjs / smoke×4） |
| P3（S9–S10） | ✅ B：plan 0 → apply 无 `--yes` **3** → apply `--yes` 0 → 二次全 skip；S10 只换 URL/Key/库 ID |
| P4（冒烟 + S11/S12） | ✅ 4 站 S8/S10 一致；S11/S12 在 shop、community PASS |
| P5（E7–E16 + 追加 P0） | ✅ 遗留目录删除、community 单库、init 三个 bug（Key 404 / agent-schedules 410 / CSV 415）修复 |
| P6（09–15） | ✅ 新增 5 个能力样例 + 2 个可选（SKIP 口径）；注册进 `run_all` 后全量回归 exit 0（默认 131 / 开备份 144 步） |

### 9.2 与计划的偏差（以实测为准）

1. **「远端 apply 无 `--yes` 退出 3」** 只在目标被判 remote 时成立；`127.0.0.1/localhost/::1/0.0.0.0` 判 local。验证约定：B 实例用 `http://localhost.:<port>`。报告的 13 条 skill 文档偏差见报告 §8。
2. **community 双数据库** 已不可用：HEAD 起单项目只允许一个 `kind=user` 数据库。已把 `media` 并入 `community` 库（`migrations/002_media.sql`）。
3. **seed 文档不可幂等**（BUG-1）：S6 拆为 S6a（不带 `--seed` 全 skip，判幂等）与 S6b（带 seed 允许重复 document，记 SKIP/PARTIAL）。
4. **`backup` 默认关闭时** `/backups` 路由不挂载（404 not_found），不是 503 `backup_disabled`；`11-backup` 两种都按 SKIP 处理，打开 `SIMPLEBASE_BACKUP_ENABLED=true` 后 14 步 PASS。
5. **计划外的 P0 样例问题**：`init.sh` 在干净实例上原本无法完成（旧 Key DELETE 404、`/agent-schedules` 410、CSV `text/csv` 415），已全部修复并在干净实例上 exit 0。
6. **§3 覆盖矩阵** 中函数触发器、实时订阅、备份、文档进阶、本地→云重放已由 P6 补齐（覆盖矩阵更新见报告 §7）。

### 9.3 未完成/未修（明确留档）

- 平台 BUG-1（集合种子不可幂等）、BUG-2（删用户数据库会 purge 项目单文件含 KV/备份）按计划"只记录不修"，修复建议见报告 §3.2/§3.3 与 §9.1。
- 旧 `examples/*-service/.env` 里 5 把 Key 指向已不存在的旧本地实例，无法吊销；文件已删除。
- 所有改动未 `git commit`（按执行约束）；提交时需按范围挑选，避免带入工作区中本会话之前已有的无关改动。
