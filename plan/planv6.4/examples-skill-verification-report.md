# examples 体检与 simplebase skill 逐站验证 —— 执行报告

> **状态：** 已执行（P1–P6，2026-11-06）
> **计划：** [`examples-skill-verification-plan.md`](./examples-skill-verification-plan.md)
> **关联改动：** `examples/**`、`scripts/examples-verify/**`（新增）、`examples/capabilities/09–15`（新增）
> **证据目录：** `/tmp/sb-verify/`（临时，可重建；关键证据已摘录在本文）

---

## 0. 结论摘要

1. **样例原有问题已全部修复**：计划列出的 E1–E6 阻断项全部处理，`examples/init.sh` 与 `examples/run_all.sh` 在 macOS 上均可从零跑通；`run_all.sh` 覆盖能力从 8 个用例扩到 15 个（01–15）。
2. **四个业务站点全部可在 SimpleBase 上跑起来**：本地实例 A + 模拟云端实例 B 双实例验证，S0–S10 全流程 **0 FAIL**；
   shop / iot-telemetry **PASS**，ops-assistant / community **PARTIAL**（唯一原因是平台缺陷 BUG-1：`dev apply --seed` 的集合种子不可幂等，已单独记录并绕过）。
   S11（变更回放）/ S12（负向用例）在 shop、community 上均 **PASS**。
3. **执行中发现 6 个计划外问题**（2 个是 P0 级、直接影响"样例能不能跑"）：
   - **BUG-0**：HEAD 引入「每个项目只允许一个用户数据库」，`community` 的双库清单直接 409，已把 `media` 并入同库；
   - **BUG-3/BUG-4/BUG-5**：`init.sh` 换实例时删旧 Key 404、`/agent-schedules` 已 410 合并到 cron、CSV 上传声明 `text/csv` 必 415，均已修复；
   - **BUG-1（平台，未修）**：集合种子文档不可幂等；
   - **BUG-2（平台，未修）**：删除用户数据库会 purge 项目单文件（KV/备份一起丢），属单文件模型的设计后果，需文档与 API 层明确。
4. **skill 文档有 13 处与实测不一致**（见 §8），主要是 local/remote 判定、Key 权限不继承、seed 文档 id 承诺、`.simplebase/env.<label>` 命名规则；建议回灌 `skills/simplebase`。

**能力覆盖**：修复 + 补齐后，计划 §3 矩阵里除「沙箱 / LLM（依赖外部条件，按设计 SKIP）」外不再有 ❌ 能力。

---

## 1. 执行结果总览

| 阶段 | 内容 | 结果 |
|---|---|---|
| **P1** | 修复 E1–E6 阻断项 | ✅ 完成。4 站 `dev validate`/`sbdev validate` 全 exit 0；iot 由失败转通过；`run_all --skip-agent` exit 0，63/63 步通过 |
| **P2** | 验证 harness | ✅ 完成。新增 `scripts/examples-verify/`（verify.sh / lib.sh / bootstrap.mjs / smoke×4） |
| **P3** | 云端重放 S9–S10 | ✅ 完成。B 实例：plan 0 → apply 无 `--yes` **3** → apply `--yes` 0（动作与 plan 一致）→ 二次 apply 全 skip；4 站 S10 只换 URL/Key/库 ID 跑通 |
| **P4** | 业务冒烟 | ✅ 完成。4 站 S8/S10 冒烟与 A 一致；S11/S12 在 shop/community PASS |
| **P5** | 清理与卫生 E7–E16（+ Lead 追加 P0） | ✅ 完成。遗留 5 个 `*-service` 删除、单库适配、init 三个 bug 修复、README/参数化全部落地 |
| **P6** | 补齐能力样例（原计划"视排期"） | ✅ 完成。新增 09-triggers / 10-realtime / 11-backup / 12-documents-advanced / 13-dev-replay；可选 14-sandbox（SKIP）、15-llm（本机 PASS，无 provider 时 SKIP） |

### 1.1 逐站判定（P2–P4，最终两轮）

| 站点 | A 本地 S0–S8 | B 云端 S9–S10 | 判定 |
|---|---|---|---|
| `shop` | 全 ✅（对象 20、KV 3、幂等下单、库存 30→29） | apply 3/0/0、二次全 skip、S10 冒烟一致 | **PASS** |
| `community` | 全 ✅（单库、posts 集合、attachments/likes/post_stats、spam 拒绝、附件删除） | 同上 | **PARTIAL**（仅 BUG-1 S6b） |
| `iot-telemetry` | 全 ✅（devices 2、rollup start/end、interval+once、worker 不重复处理） | 同上 | **PASS** |
| `ops-assistant` | 全 ✅（tickets、只读 Key 14 步、SLA overdue、agent schedule=target_kind agent） | 同上 | **PARTIAL**（仅 BUG-1 S6b） |

> 报告：全量 4 站 `/tmp/sb-verify/harness/20261106-180515-47112/report.md`（76 行，0 FAIL，6 SKIP）；
> S11/S12 `/tmp/sb-verify/harness/20261106-180559-56879/report.md`（shop PASS / community PARTIAL，S11、S12 均 PASS）。
> SKIP 仅两类：BUG-1（S6b）与「无 LLM provider → 助手/助手定时未验证（S8-llm）」，均按计划 §4.6 不计 FAIL。

### 1.2 全量 run_all 回归（新增 09–15 注册后，干净实例）

| 场景 | 命令 | 结果 |
|---|---|---|
| 默认配置（`backup.enabled=false`） | `./examples/run_all.sh --start --skip-build --skip-agent --api-port 18086 --fresh` | **exit 0**，`summary ok=131 fail=0 total=131`；11-backup 正确 SKIP（`/backups` 路由未挂载 → 404） |
| 打开备份（`SIMPLEBASE_BACKUP_ENABLED=true`） | 同上（端口 18087） | **exit 0**，`summary ok=144 fail=0 total=144`；11-backup 14 步全 PASS |

用例分布：01(8) 02(9) 03(12) 04(9) 05(7) 06(8) 07(5) 08(7) 09(15) 10(13) 11(1|14) 12(20) 13(12) 14(1 SKIP) 15(4)。

---

## 2. P1：阻断项修复与验收（E1–E6）

| ID | 问题 | 修复 | 验收证据 |
|---|---|---|---|
| E1 | `date +%s%3N` 是 GNU 扩展，macOS BSD date 输出 `...3N`，算术展开直接退出 | `capabilities/lib/common.sh`、`run_agent.sh` 增 `now_ms()`（python3） | 01–08 在 macOS 全 PASS（修复前第一步即失败） |
| E2 | `PATH` 硬编码 `/home/ubuntu/.nvm/...` | 删除，改 `command -v node` 检查 + 提示；仅 agent suite 需要 node | `run_all --skip-agent` 不再要求 node |
| E3 | `iot_export_once` 的 `once` 无 `run_at`，靠运行时补 → skill/`dev validate` 拒绝 | 清单显式 `run_at=2099-01-01T00:00:00Z`；删 `run.mjs` 运行期补写 | iot validate 由 **exit 1 → exit 0**（双入口一致） |
| E4 | agent cron 用例的 `atick<RUN_ID>` 函数不存在，服务端必 `gofunction_not_found` | `agent/run.mjs` 用例前用 CLI `function create`（复用 08-cron/tick.go），失败记 setup FAIL 并跳过 | 08-cron 的 create/invoke/trigger/runs 实测通过；逻辑 fail-closed |
| E5 | 默认 8080 被占（本机 nginx），health 失败提示误导 | `--api-port`（监听 + URL 同源）、`--skip-build`、health 失败打印真实状态码 | 服务端日志 `"address":"127.0.0.1:18280"`；两次验收均 0 失败 |
| E6 | 清单不写 `data_model`：skill 推断 `sql`、init 强制 `collection` → 同实例 `data_model_mismatch` | 4 清单显式 `"data_model":"collection"`；删 `run.mjs` 的 derived 覆盖 | **两种入口交替 apply 全 skip、无 `data_model_mismatch`**（S6 实测） |

**P1 验收命令结果**

```text
dev validate   shop=0 community=0 iot-telemetry=0 ops-assistant=0
sbdev validate  shop=0 community=0 iot-telemetry=0 ops-assistant=0
run_all --start --skip-build --skip-agent --api-port 18280  → exit 0, summary ok=63 fail=0
python3 -m unittest discover -s skills/simplebase/scripts -p 'test_sbdev.py' → 28 tests OK
```

---

## 3. 计划外发现（按严重度）

### 3.1 BUG-0（P0，已修样例）：单项目单库约束打破 community 双库设计

- **事实**：HEAD `b6e9936` 在 `internal/catalog/service.go` 引入 `HasDefaultDatabase`，每个项目只允许一个 `kind=user` 数据库；`docs/data/sql.md` 已同步为「每个项目只有一个数据库」。
- **实测**：干净实例上 `dev apply --dir examples/community` →
  `{"error":{"code":"database_already_exists","message":"database media 失败：database already exists"}}`。
- **修复**：`media-migrations/001_init.sql` → `migrations/002_media.sql`（`git mv`，内容不变），manifest 只留 `community` 库；README 改为「单库同时承载 posts 集合 + attachments/likes/post_stats 表」。
- **验收**：ex-cmty1 上 1 个库；表 `attachments/likes/post_stats/posts` 齐全；actions: create database → 001 → 002 → seed → collection → document → function → cron。

### 3.2 BUG-1（P0 平台，未修）：`dev apply --seed` 的集合种子不可幂等

- **事实**：服务端 `CreateDocument` 永远生成 UUID 并忽略传入的 `id`（`docs/data/sql.md` 也写明 id 由服务端生成），而 `internal/sbcli/dev.go` 用 `GetDocument(seed.id)` 判重 → 判重永远落空。
- **实测**：ops-assistant 第二次 `dev apply --seed`：其余全 skip，但 `create document ops/ticket_meta/ticket-demo`，`changes=1`；community 的 `posts/demo` 同理。
- **与文档冲突**：`skills/simplebase/references/manifest.md` 承诺「seed 文档按 id 去重」。
- **绕行**：S6 拆成 S6a（第二次 apply **不带** `--seed` → 全 skip，作为幂等判据）与 S6b（带 seed 时允许只多 document，记 PARTIAL）。用例 13 明确不依赖 seed 文档 id。
- **建议修复方向**（二选一）：
  1. 平台允许创建文档时携带合法 `id`（PocketBase 风格），`dev.go` 的判重即刻生效；或
  2. `dev apply` 用 `_migrations` 同款记账（`seed-doc:<db>/<collection>/<id>`），一次性写入、重复 skip；两端（Go/Python）实现需保持同一语义与 fixture。

### 3.3 BUG-2（P0 平台，未修）：删除用户数据库 = 清空项目单文件

- **事实**：单文件模型下 SQL / 集合 / KV 共用一个 `{project}/database.duckdb`；`catalog.DeleteDatabaseSync → projectPurger.PurgeProjectFiles` 会删除它（`internal/app/project_purger.go` 注释只承诺保留 `files/`）。
- **实测**：`create db → delete db` 后 KV SET 仍返回 OK，但随后 `backup create` 500 `internal_error`（`filedb: open snapshot source: ... no such file`）；同一实例内备份/快照不可用，重启前 KV 数据实际丢失。
- **影响**：能力用例 03/04 结尾删库会把同项目 KV/备份一起清掉；11-backup 需自愈「新建专用项目」。
- **建议**：删库 API 明确语义并在文档 + 控制台二次确认中写清「会删除本项目全部 SQL/集合/KV 数据」；或单文件模型下把「删库」改成「清空数据」而非 purge 文件。

### 3.4 其余已修复问题

| ID | 问题 | 证据 | 修复 |
|---|---|---|---|
| BUG-3 | `init.sh` 换实例时按 `.env` 里旧 Key ID DELETE → 404 `not_found`，init 中断 | `initial: DELETE /api-keys/...: HTTP 404` | 旧 Key 删除容忍 404（顺序仍 签发→写 .env→删旧） |
| BUG-4 | `/agent-schedules` 已合并，POST 返回 410 `agent_schedules_merged` | `init: POST /agent-schedules: HTTP 410` | 改 `/cron-jobs` + `target_kind=agent`（按 agent_id 幂等 create/PATCH） |
| BUG-5 | CSV 对象声明 `text/csv`，服务端 `sniffUploadMIME` 嗅探为 `text/plain` → 415 | `POST /s3/objects: HTTP 415 mime_mismatch` | `run.mjs` csv 改 `text/plain`；harness 一律 `application/octet-stream` |
| — | `11-backup` 在完整 run_all 中 preflight 失败 | `backup list: not_found`（备份关闭时路由未挂载） | 404/`backup_disabled` 均按「未启用」SKIP；打开 `SIMPLEBASE_BACKUP_ENABLED=true` 时 14 步全 PASS |

### 3.5 计划断言与实测的差异（需修计划/文档，不是 bug）

| 计划原文 | 实测 |
|---|---|
| shop `GET /api/products` 返回 `{products:[...]}` | 返回**顶层数组**；字段 `sku/name/price_minor/stock/image_key/image_url`（image_url 需对象种子后非 null） |
| "B 上 apply 不带 --yes 退出码 3" | 前提是目标被判 remote；`127.0.0.1/localhost/::1/0.0.0.0` 都判 local，**不会**要 `--yes`。验证约定：B 用 `http://localhost.:<port>`（尾点，解析到 127.0.0.1 但 host 字符串不等） |
| community "两库 community、media" | 平台已单库；已合并为 1 库（BUG-0） |
| ticket_meta 集合含 id=`ticket-demo` | 文档 id 由服务端生成 UUID；`ticket-demo` 是 SQL `tickets` 表的 id，断言应改按内容（owner=demo-user） |
| ops 健康检查 `/api/health` | `/health` 与 `/api/health` 都存在（两者均 200） |
| 项目 ID 任意 | 必须**恰好 8 位** `[A-Za-z0-9-]`（`ex-mini2` 7 位会失败） |
| 发布 Key 权限可叠加 | 权限**不继承**：list/query 要 `database:read`、写要 `database:write`、建库要 `database:admin`，发布 Key 必须同时持有三者 |

---

## 4. P2–P4：验证 harness 与逐站执行

### 4.1 交付物

```text
scripts/examples-verify/
  verify.sh        # 入口：--only / --skip-cloud / --with-changes / --keep / --ports / --repo
  lib.sh           # now_ms、redact、record(jsonl {suite,case,env,step,ok,ms,detail,status})、health 等待、端口检查
  bootstrap.mjs    # 补位：登录、同 ID 建项目、签发 publish(admin+read+write)/runtime(read+write)、
                   #   .env/.env.cloud/.env.runtime（0600）、kv/objects 种子、trigger_once、agent schedule、多库 env
  smoke/{shop,community,iot-telemetry,ops-assistant}.sh
```

- 两个实例：A=本地、B=模拟云端（不同端口/数据/缓存目录，全部落 `/tmp/sb-verify/<RUN_ID>/`）。
- 工作区先 `cp -R examples/<case>` 到临时副本，skill 产生的 `.simplebase/`、`.env*` 不落仓库。
- B 实例 URL 固定 `http://localhost.:<port>` 以覆盖 remote 分支；S9→S10 之间由 bootstrap 给 B 补 kv/objects/trigger_once（「数据不搬」边界）。
- 判定：PASS / PARTIAL（有 SKIP 无 FAIL）/ FAIL；FAIL→退出 1，仅 PARTIAL→退出 0 + 警告；LLM/沙箱缺失一律 SKIP。

### 4.2 步骤与结果

| 步骤 | 内容 | 结果 |
|---|---|---|
| S0–S1 | 副本 + 双入口 `validate` 对账 | 4 站全 ✅（warning 集合一致） |
| S2–S5 | bootstrap、doctor、plan、apply | 4 站全 ✅（plan=apply 同动作序列） |
| S6a | 二次 apply（不带 seed）+ CLI 交叉 apply | 4 站全 skip、无 `data_model_mismatch` ✅ |
| S6b | 二次 apply（带 seed） | ops/community 各多 1 条 seed document → SKIP/PARTIAL（BUG-1），shop/iot ✅ |
| S7 | 资源断言（表/集合/函数/cron/once） | 4 站 ✅ |
| S8 | 补种子 + BFF/脚本冒烟 | 4 站 ✅ |
| S8-llm | 助手/助手定时 | SKIP（harness 默认不注入 provider） |
| S9 | B 重放：plan 0 → apply 无 --yes **3** → apply --yes 0 → 二次全 skip → CLI remote 交叉全 skip | 4 站 ✅ |
| S10 | 只换 URL/Key/库 ID 在 B 跑同一冒烟 | 4 站 ✅ |
| S11 | 新增迁移 + 更新函数 → A apply → B plan/apply | shop、community ✅ |
| S12 | 改已应用迁移 → `migration_modified` 退出 1；函数假 `sb_live_` → validate 拒绝 | shop、community ✅ |

### 4.3 harness 自身修掉的问题

S7 jq 多包一层 `[...]`、对象上传 MIME、node BFF 必须相对路径启动（macOS `/tmp`→`/private/tmp` 使 `import.meta.url` 守卫为 false）、Go BFF 需独立 `GOCACHE/GOTMP`、bash 3.2 中文标点并进变量名、S11 迁移编号与 `002_media.sql` 冲突。

---

## 5. P5：清理与卫生（E7–E16）

| ID | 结果 |
|---|---|
| E7 | 删除 5 个未跟踪遗留目录 `examples/{booking,community,mini-game,shop,ticket}-service`（记录 URL=`127.0.0.1:8080`、project=`6n79jc0h`；旧本地实例已不存在，**5 把 sb_live_ Key 无法吊销**，目录已删） |
| E8 | `keys()` 按 manifest 写多库 env（`SIMPLEBASE_DATABASE_ID_<NAME>`，首库保留无后缀） |
| E9 | `resetSeeds` 遍历该库 migrations 目录全部 `*.sql` 推导表名（去重/逆序/缺表容错） |
| E10 | 删除 `examples/lib/goexample/cmd/build`（tracked 删除） |
| E11 | `run_agent.sh` 先 GET `/agents` 校验内置助手；缺失时整体 SKIP（ok=true）不 FAIL |
| E12 | shop README「实时云函数」→「下单时同步调用 `pricing.Quote` 算价」 |
| E13 | 06/08 用例末尾清理函数；01-project 复用 `PROJECT_ID_FILE`、`--fresh` 才新建；`run_all` 增 `--fresh` |
| E14 | 4 站 README 验证清单与实际资源对齐；iot 注明 once `run_at=2099` |
| E15 | `examples/README.md` 逐条复核 G0b–G16：G0/G3/G9/G14 已修复或由 `dev` 补位；G5 按决策缓解；仍适用 G0b、G1、G2、G4、G6、G7、G8、G10–G13、G15、G16 |
| E16 | 未改 `.gitignore`/`config.yaml`；验证只在临时副本进行；仓库内无新增 `.simplebase/` |

**P5 验收**

```text
clean instance (18281): ./examples/init.sh --url http://127.0.0.1:18281 --skip-build → exit 0
  [iot-telemetry]/[shop]/[community]/[ops-assistant] ready；objects shop=20 / community=1 / iot=1
  kv 每项目 2 条；ops cron 出现 ops-daily-schedule target_kind=agent；无 404/410/415
run_all 两轮（新建/复用）exit 0，summary ok=65 fail=0
```

---

## 6. P6：新增能力样例（09–15）

| 用例 | 内容 | 实测（P6 自测 + 全量 run_all） |
|---|---|---|
| `09-triggers` | record_change / http invoke 触发器、触发记录、enabled 切换、队列 dropped | 15 步 PASS（sink 恰好 1 次回调、invoke 202、dropped=0） |
| `10-realtime` | SSE create/update/delete、订阅更新、`Last-Event-ID` 回放 | 13 步 PASS |
| `11-backup` | create → KV 覆盖 → restore → download → delete；KV 快照恢复 | 备份开启时 14 步 PASS；关闭时 SKIP |
| `12-documents-advanced` | patch/batch/schema/filter+fields/export→import/truncate | 20 步 PASS（required_field、3 行往返） |
| `13-dev-replay` | 最小工作区 dev validate/plan/apply ×2；skill 对账 | 12 步 PASS（第二次 changes=0 全 skip） |
| `14-sandbox` | 可选沙盒 | SKIP（macOS `sandbox-exec: Operation not permitted`） |
| `15-llm` | `/v1/models`、chat（含流式）、无 Key 401 | 本机有 provider → 4 步 PASS；provider 关闭实例上 SKIP |

- `examples/capabilities/README.md` 增 09–15 主表 + 详细表（命令主线/断言/前置与 SKIP/资源影响）+ 推荐注册顺序 + 公共约定。
- `examples/run_all.sh` 的 cases 已注册为：
  `01-project 02-auth 11-backup 03-sql 04-documents 05-kv 06-functions 07-storage 08-cron 09-triggers 10-realtime 12-documents-advanced 13-dev-replay 14-sandbox 15-llm`
  （11 在 03/04 之前，避免「删库清空项目文件」影响备份；14/15 可选放最后）。
- Lead 修复：`11-backup` 在备份关闭时 preflight 404 → 改为 SKIP（打开时全 PASS）。

---

## 7. 更新后的能力覆盖矩阵

来源：`internal/sbcli` 命令表、`internal/api` 路由、`docs/`、`examples/capabilities`。
✅ 有可运行样例且有断言；◐ 间接使用/无独立断言；❌ 无样例。

| 能力域 | 子能力 | 覆盖 | 样例 |
|---|---|---|---|
| 项目 | 创建/列表/设置/配额/审计/日志/日志保留 | ✅ | 01-project |
| 用户与鉴权 | 建用户、只读 Key、写拒绝、吊销 | ✅ | 02-auth、ops verify-perms |
| SQL 库 | 建库/建表/加列/CRUD/schema | ✅ | 03-sql、4 站点 |
| SQL 事务 | `sql batch --transactional` | ✅（站点间接） | shop 下单、iot worker、12 |
| SQL 迁移 | `dev apply` up-only + `_migrations` | ✅ | 13-dev-replay |
| 文档 | 集合/增删改查 | ✅ | 04-documents、community/ops |
| 文档进阶 | patch/batch/schema/filter/export/import/truncate | ✅ | **12-documents-advanced** |
| KV | String/Hash/INCR/LIST/SET/ZSET/SCAN/DEL | ✅ | 05-kv、4 站点 |
| KV TTL | EX/TTL | ✅ | iot `online:*`、05 |
| 云函数 | 创建/调用/版本/试跑/激活/删除 | ✅ | 06-functions、4 站点 |
| 定时任务 | cron/interval/once、触发、runs、删除 | ✅ | 08-cron、4 站点 |
| 对象存储 | 上传/列举/预签名/删除 | ✅ | 07-storage、shop/community/iot |
| 函数触发器 | record_change / http invoke | ✅ | **09-triggers** |
| 实时订阅 | SSE、订阅更新、Last-Event-ID | ✅ | **10-realtime** |
| 备份恢复 | create/list/download/restore/delete | ✅ | **11-backup** |
| 云助手 | SSE 运行、确认、只读拦截、agent schedule | ✅（provider 缺失时 SKIP） | `capabilities/agent`、ops manifest |
| 沙箱 | create/connect/pause/delete、E2B 兼容 | ◐（按设计 SKIP） | **14-sandbox** |
| LLM | OpenAI 兼容 models/chat/stream | ✅（provider 缺失时 SKIP） | **15-llm** |
| SDK | JS SDK / Go SDK | ✅（站点间接） | shop/iot、community/ops |
| 本地→云重放 | dev validate/plan/apply/status、sbdev.py | ✅ | **13-dev-replay** + verify.sh S9/S10 |

---

## 8. skill 文档与实测不一致（建议回灌）

1. `SKILL.md` 只写「远端 apply 无 `--yes` 退出 3」，没写 **`127.0.0.1`/`localhost` 会被判 local**；验证 remote 分支需要非 localhost host（本报告用 `localhost.`）。
2. 发布 Key 权限**不继承**：`database:admin` 不能代替 `database:read/write`；文档需给出最小权限组合。
3. `.simplebase/env.<label>`：label 取自 URL hostname（本地固定 `local`，远程为 hostname）；**单库**才写不带后缀的 `SIMPLEBASE_DATABASE_ID`，多库只有 `_<NAME>` 行。
4. `references/manifest.md` 的「seed 文档按 id 去重」目前不可实现（BUG-1），文档应注明限制或待平台修复后再承诺。
5. `references/manifest.md` 未提示单项目单库约束（BUG-0），应写明「一个项目只能有一个用户数据库」。
6. 对象上传 MIME 规则（声明 vs 嗅探 vs 扩展名）应在 `references/cli.md` 或 storage 文档中说明；CSV 应声明 `text/plain`。
7. `doctor`/`apply` 对「项目已是旧实例/旧 .env」的换环境场景（旧 Key 404、旧库类型不一致）缺少提示。
8. `iot` 类脚本的启动口径：Node 相对路径启动 vs 绝对路径（`/tmp`→`/private/tmp`）会导致 `import.meta.url` 守卫静默不执行，README 示例没问题但需注明。
9. `backup` 默认关闭时 `/backups` 路由不挂载（404），不是 503 `backup_disabled`；SKIP 判定要包含 404。
10. `project create` 只能在控制台指定 ID？CLI 无 `--id`（skill 要求两端同 ID 时只能走 HTTP 或控制台）——建议补 CLI 能力。
11. `delete database` 的破坏范围（同项目 SQL/集合/KV/备份）文档未写明。
12. `simplebase dev apply` 会向 `--dir` 写 `.simplebase/`，对仓库内 examples 直接执行会留未跟踪目录；建议文档提示用副本或在示例里加 gitignore。
13. LLM provider 与助手：`requires.llm` 的站点在无 provider 时哪些功能不可用（agent、summary）应集中说明。

---

## 9. 优化建议

### 9.1 平台（按优先级）

1. **修 BUG-1（seed 文档幂等）**：允许客户端指定文档 `id`（并校验唯一），或在 dev replay 用 `_migrations` 同款记账；这是 skill「幂等重放」承诺的最后一块。
2. **修 BUG-2（删库语义）**：单文件模型下删除用户数据库会连带 KV/备份。至少 API 文档 + 控制台确认文案写清；更好的做法是「删库=清空数据」而非 purge 文件，避免误删不可恢复。
3. **`project create --id` 进 CLI**：skill 要求两端同 project ID，现在只能控制台/HTTP；补上后 `bootstrap` 与 `init.sh` 可完全 CLI 化。
4. **权限文档/校验**：`database:admin` 是否隐含读写，明确写出；或在 `auth.Principal.HasPermission` 做层级继承（需评估兼容性）。
5. **备份默认开关**：CI/示例若要覆盖备份，`run_all` 需要显式打开 `SIMPLEBASE_BACKUP_ENABLED=true`；建议 `examples/README` 写明。

### 9.2 工程/CI

1. 把 `scripts/examples-verify/verify.sh --skip-cloud` 接入 Linux CI，macOS runner 每周一次（防 E1 类平台差异回归）；`dev validate` 四站作为快速门禁。
2. 把 `examples/capabilities/{01..15}` 的 `bash -n` + `run_all --skip-agent`（默认配置）作为 PR 必跑。
3. 统一 result schema：capabilities / agent / examples-verify 已用同一 jsonl 字段（`suite/case/env/step/ok/ms/detail/status`），建议再抽一个公共 `record` 实现。
4. `examples/init.sh` 与 `scripts/examples-verify/bootstrap.mjs` 逻辑重复（建项目/签 Key/种子），P2 完成后可让 `init.sh` 复用 bootstrap。
5. 样例 README 模板化：每站固定「运行 / 验证清单 / 已知缺口 / 用 skill 重放」四节，最后一节直接给 S1–S10 命令（verify.sh --only <case>）。

### 9.3 样例

1. community 单库后，`examples/README`、各站 README、`planv4.0` G 编号引用已同步；后续平台若恢复多库，需重新评估。
2. `11-backup` 依赖 `backup.enabled`；README/CI 中明确「默认 SKIP，开备份后 PASS」。
3. iot/ops 的「助手」与「summary」在无 LLM provider 时是 SKIP，不是失败——README 已写明，CI 判定需保持同一口径。

---

## 10. 验收对照（计划 §8）

| # | 验收项 | 结果 |
|---|---|---|
| 1 | macOS/Linux `run_all --start --skip-agent` 全绿 | ✅ macOS：默认配置 exit 0 / 131 步，开备份 exit 0 / 144 步 |
| 2 | 4 站 `dev validate` 与 `sbdev.py validate` 均退出 0、warning 一致 | ✅ 双入口 0/0，iot 由失败转通过 |
| 3 | `verify.sh` 4 站 PASS 或 PARTIAL，report.md 给出 S1–S10 | ✅ shop/iot PASS，ops/community PARTIAL（仅 BUG-1 + LLM SKIP），**0 FAIL** |
| 4 | S6：两种入口交替 apply 全 skip、无 `data_model_mismatch` | ✅ S6a 实测 |
| 5 | S10：只改 URL/Key/库 ID 即可在 B 跑通 | ✅ 4 站 S10 冒烟与 A 一致 |
| 6 | `examples/` 下不再有未跟踪 `.env` Key 文件 | ✅ 5 个 `*-service` 已删；旧 Key 无法吊销已记录 |
| 7 | 覆盖矩阵中 ❌ 只剩可选的沙箱/LLM | ✅ 09–13 补齐；14/15 按设计 SKIP |
| 追加 | 09–15 注册进 `run_all` 且全量回归 | ✅ 见 §1.2 |

---

## 11. 遗留风险与后续待办

1. ~~**BUG-1 / BUG-2 未修**~~ BUG-1 已修（§13）；BUG-2 保留为单文件模型的设计语义，已在 skill / CLI 文档写明删库范围。
2. **旧 Key 无法吊销**：`examples/*-service/.env` 中 5 把 Key 指向已不存在的 `127.0.0.1:8080` 旧实例；文件已删，若该实例曾暴露到公网需在对应环境吊销。
3. **LLM/沙箱用例**依赖外部条件，CI 中应保持 SKIP 口径；`14-sandbox` 在具备 bwrap+Landlock 的 Linux 上应复跑一次。
4. **未提交**：本次改动全部在工作区，未 `git commit`；`git status` 中另含本会话开始前已有的无关修改（`internal/`、`godatabase/`、`ui/` 等），提交时需按范围挑选。
5. **`planv6.3/pocketbase-capability-gap-plan.md` 与本文覆盖矩阵**存在部分重叠，后续合并时以本文 P6 结果为准。

---

## 12. 证据索引

| 内容 | 路径 |
|---|---|
| 全量 4 站 S0–S10 报告 | `/tmp/sb-verify/harness/20261106-180515-47112/report.md` |
| shop/community S11–S12 报告 | `/tmp/sb-verify/harness/20261106-180559-56879/report.md` |
| 全量 run_all（默认配置） | `/tmp/sb-verify/lead/final3-runall.log`、`final3-results.jsonl` |
| 全量 run_all（开备份） | `/tmp/sb-verify/lead/final4-runall.log`、`final4-results.jsonl` |
| P1 验收 | `/tmp/sb-verify/fixer/results.jsonl`（63 步） |
| init.sh 干净实例验收 | `/tmp/sb-verify/fixer/init-clean.out` |
| P6 自测 | `/tmp/sb-verify/coverage/qa-results.jsonl`（79 行，0 fail） |

> `/tmp` 为临时目录；`scripts/examples-verify/` 可随时重跑重建以上证据：
> `scripts/examples-verify/verify.sh --ports 18090,18091`、
> `SIMPLEBASE_BACKUP_ENABLED=true ./examples/run_all.sh --start --skip-build --skip-agent --api-port 18087 --fresh`。

---

## 13. 报告问题的代码优化（2026-11-07）

| 来源 | 改动 | 文件 |
|---|---|---|
| BUG-1 集合种子不可幂等 | 种子文档不再按 id 查重（服务端忽略 id），改为在 `_migrations` 记 `seed-doc:<集合>/<id>`；Go 与 Python 两端同语义 | `internal/sbcli/dev.go`、`skills/simplebase/scripts/sbdev.py` |
| BUG-1 测试 | 假服务端/假 CLI 模拟「忽略 id」；新增二次 `--seed` 不重复建文档断言 | `internal/sbcli/dev_test.go`、`skills/simplebase/scripts/test_sbdev.py` |
| BUG-1 harness | S6b 改为严格断言（二次 `--seed` 必须全 skip）；S9 二次 apply 带 `--seed`；删除 BUG-1 绕过注释 | `scripts/examples-verify/verify.sh`、`smoke/community.sh` |
| BUG-1 样例 | `13-dev-replay` 工作区加入集合种子，断言 collection/document 动作 | `examples/capabilities/13-dev-replay/{workspace/simplebase.json,run.sh,README.md}` |
| BUG-0 单项目单库 | validate 在声明多于 1 个库时警告（两端文案一致，fixture 同步） | `dev_workspace.go`、`sbdev.py`、`testdata/dev/basic/expect.json` |
| 清单测试 | `TestDevExamplesManifestsLoad` 改为：零错误、恰好 1 库、显式 `collection` | `internal/sbcli/dev_test.go` |
| §8-10 / §9.1-3 | `project create --id`（CLI）+ `CreateProjectWithID`（Go SDK） | `internal/sbcli/commands.go`、`packages/go-sdk/resources.go` |
| BUG-5 / §8-6 | 上传 MIME：声明 `text/csv`、`application/json` 等纯文本子类型而嗅探为 `text/plain` 时接受；`text/html` 等仍拒绝 | `internal/api/s3_content_handler.go`（+ 测试） |
| §8-8 | ESM 入口守卫改用 `realpathSync` 比较，相对路径与 `/tmp`→`/private/tmp` 都能启动 | `examples/shop/server/main.mjs`、`examples/iot-telemetry/scripts/worker.mjs` |
| §8-1/2/3/4/5/7/11/12 | skill 文档回灌：local 判定与 `localhost.`、权限不继承、env label 规则、种子记账、单库、换环境旧 Key、删库范围、用副本验证 | `skills/simplebase/{SKILL.md,references/manifest.md,references/cli.md}`、`docs/cli.md` |
| §9.2-1 | `scripts/test.sh` 增加四站 `sbdev.py validate` 快速门禁 | `scripts/test.sh` |

**复验：**

- `go test ./internal/sbcli/ ./packages/go-sdk/` 通过；`internal/api` 的 S3 用例通过（`TestQuotaHandler`/`TestAuditHandler` 失败来自工作区里先前未提交的 `ErrProjectMissing` 改动，与本次无关）。
- `python3 -m unittest … test_sbdev.py`：29/29 通过。
- `run_all.sh --start --skip-build --skip-agent --api-port 18087 --fresh --only 01-project,13-dev-replay`：20/20 PASS。
- `verify.sh --ports 18090,18091`：4 站 S6b 全部 PASS（二次 `--seed` 全 skip）；结果 shop/iot PASS，ops/community PARTIAL **仅因无 LLM provider 的 S8-llm SKIP**，0 FAIL。报告：`/tmp/sb-verify/harness/20261107-072620-68948/report.md`。

**未处理：** ~~§9.1-2 删库语义、§9.1-4 权限层级继承~~ 已在 §14 处理。

---

## 14. 遗留项处理（2026-11-07 下午）

| 来源 | 方案 | 改动 |
|---|---|---|
| BUG-2 / §9.1-2 删库=清空数据 | `catalog.DeleteDatabaseSync` 新增 `ProjectDataClearer` 平面并优先使用：以 ReadWrite 租约删除用户表、视图、序列（跳过 `__*` 内部表，即 KV），再清理系统库里该项目的集合元数据；项目文件不删除，备份 / KV / 快照同步照常。没有 registry 时退回旧的整体删文件 | `internal/catalog/service.go`、`internal/app/project_purger.go`（重写）、`internal/app/app.go` |
| §9.1-4 权限层级 | `auth.PermissionImplied`：`database:admin` 隐含 `database:write`、`database:read`；`write` 不隐含 `read`；鉴权（`HasPermission`）与签发子集校验（`limitIssuedPermissions`）共用同一规则 | `internal/auth/principal.go`、`internal/api/api_keys_handler.go` |
| `internal/api` 两个失败用例 | `ErrProjectMissing` 已映射为 400 `invalid_request`（工作区既有改动），测试期望由 500 改为 400 | `internal/api/quota_audit_handler_test.go` |
| 文档 | 删库影响、权限继承口径同步 | `skills/simplebase/{SKILL.md,references/cli.md}`、`docs/getting-started/{concepts,local-cloud-collab}.md`、`examples/capabilities/{README.md,13-dev-replay/README.md}` |

**测试：**

- 新增 `internal/app/project_purger_test.go`：DuckDB / SQLite 两种引擎下删库后用户表与视图消失、`__sb_kv_strings` 数据保留、集合元数据清理；无 registry 时只暴露删文件平面。
- 新增 `internal/auth` `TestPermissionHierarchy`。
- `go test` `internal/{app,auth,catalog,api,sbcli,agentbridge}`、`packages/go-sdk` 全部通过（`internal/api` 已无失败）。
- 端到端（真实实例，开备份）：建库 → 建表 → KV SET → 删库 → KV GET 仍为 `alive`、`backup create` 成功、重建库后旧表不存在。
- `SIMPLEBASE_BACKUP_ENABLED=true run_all.sh --skip-agent --fresh`：150/150 PASS（含 09–15）。
- `verify.sh --with-changes`：0 FAIL；shop PASS，其余 PARTIAL 仅因无 LLM provider 的 S8-llm 与按设计只在 shop/community 跑的 S11/S12。报告：`/tmp/sb-verify/harness/20261107-170200-79460/report.md`。

**兼容性：** 只持有 `database:admin` 的既有 Key 现在可以读写（之前会 403）；`gocloudagent` 的只读降级仍同时剥离 `write` 与 `admin`，不会因继承获得写权限。删库后同项目的 KV 不再丢失，依赖「删库清空 KV」的脚本需改为显式 `kv exec --command DEL`。
