# examples 业务案例重写计划（v3.1）

> 状态：待实施。本文件只做规划，不改代码。
> 依据：`docs/`、`internal/api/router.go` 等当前实现（2026-10 核对）。文中只要写“现状”，都以代码为准；做不到的能力集中写在 §6「平台缺口」，不能当成已上线功能。
>
> v3.1 评审结论（2026-10-02）：
> - **G0 先修复**：项目归属校验是第一个里程碑（M1），修完才开始写案例。
> - **G3 本轮处理**：KV 路由按命令区分读写权限。
> - **G6 去除依赖**：案例 DDL 不写 `PRIMARY KEY` / `UNIQUE`，唯一性由应用层保证；文档写明这一限制。
> - **G7 接受现状**：不做删除项目，出问题时整实例重置（`./build.sh reset -y`）。
> - **G5 定为超管登录**：init 只用超管账号登录，不再使用 DevMode 种子 Key。

## 1. 背景与目标

### 1.1 现有 `examples/` 的问题

| # | 问题 | 现状证据 |
|---|---|---|
| P1 | 场景重复，没有验证“开发习惯” | 5 个案例（shop/booking/ticket/mini-game/community）都是“BFF + 1 个 `Tick/Compute` 函数 + 1 个 cron + KV 缓存”同一套路，ticket 与 booking 几乎一样，读完一个就没有新信息。 |
| P2 | 功能覆盖不全 | 没有一个案例用到：S3 业务文件（只上传 dist）、预签名下载、对象删除、interval / once 定时、函数版本回滚、API Key 最小权限、LLM、Cloud Agent / agent 定时、日志 / 指标 / 审计 / 配额。 |
| P3 | `init.sh` 不创建项目 | 所有案例挤在 DevMode 种子项目 `dev-shop`，靠库名、`ex_<case>_` 前缀隔离；控制台里看不到“一个场景一个项目”的效果。 |
| P4 | 初始化后控制台是空的 | 表要等 BFF 首次启动 `CREATE TABLE IF NOT EXISTS` 才出现；没有种子数据、KV、对象和激活的函数，打开控制台什么都看不到。 |
| P5 | 平台限制写在各 README 里 | 有的标在 README，有的写在旧 plan，没有统一缺口清单，也没进 backlog。 |

### 1.2 目标（对标 Supabase examples 体验）

1. **4 个场景，每个场景一个独立 SimpleBase 项目**，合起来覆盖 §3 矩阵列出的全部对外功能。
2. **`./examples/init.sh` 一条命令**：创建项目 → 建库 → 跑迁移和种子 → 写 KV → 上传对象 → 发布并激活云函数 → 建定时任务（及 agent）→ 签发各项目最小权限 Key → 生成 `.env` → 自检。执行完打开控制台切到对应项目，**不启动任何案例代码**就能看到表、数据、KV、文件、函数、任务和运行记录，并可直接在控制台的 SQL / KV / 云函数 / 定时任务页面上手验证。
3. 案例代码按真实开发习惯组织：迁移文件、种子文件、SDK 调用、可信 BFF、单文件云函数、清单驱动部署；不靠 init 内的魔法步骤。
4. 做不到或不顺手的地方统一记入 §6，每条注明现状、案例里怎么绕、建议的平台改进。

### 1.3 非目标

- 服务端只改 §6.1 列出的两项（G0、G3），其余缺口只记录、另立 plan；不改 SDK 和对外协议；不新增第三方依赖；不改 `package.json` / `go.mod`。
- 不提供生产部署（HTTPS、域名、限流、多实例）；BFF 只监听 `127.0.0.1`。
- 不做压测资产（已在 `benchmarks/gofunction/`）。

## 2. 平台能力基线（按代码核对）

| 能力 | 接口（前缀 `/v1/projects/:projectID`，除非另注） | SDK | 关键约束 |
|---|---|---|---|
| 登录 | `POST /v1/auth/login`、`/v1/auth/refresh`（免认证） | 无 | access 2h；同 IP+用户名 10 次/分钟限流；种子超管 `simplebase2026/simplebase2026`（不强制改密）；超管 JWT 每次请求都会重新展开项目集，新建的项目可以立即访问 |
| 项目 | `GET/POST /v1/projects` | 无 | 创建需 `project:admin`；可指定 `id`，必须恰好 8 位 `[A-Za-z0-9-]`；**没有删除接口** |
| API Key | `POST/GET/DELETE /api-keys` | 无 | 只绑定一个项目；签发要求登录态 super/user，或**本项目**的 `project:admin` Key；明文只返回一次 |
| 用户库 | `POST/GET/DELETE /databases` | JS `databases`、Go `CreateDatabase` 等 | 名称 ≤63 字节，`kv` 保留；系统库只读 |
| SQL | `/databases/:id/query`、`execute`、`batch` | JS `sql.query/execute/batch`、Go `Query/Execute/Batch` | 参数 `?`；batch 可事务；**DuckLake 不支持 PRIMARY KEY / UNIQUE / 索引 / 序列 / 非字面量 DEFAULT**，upsert 只能 `MERGE INTO`；案例一律不声明约束（§6 G6） |
| 文档集合 | `/databases/:id/data/collections[/:c[/documents[/:id]]]` | JS `collection()`、Go documents | 集合名 `[A-Za-z_][A-Za-z0-9_]*`；与 SQL 不在同一事务 |
| 项目 KV | `POST /kv`（`cmd` 或 `String/Hash/List/Set/ZSet`） | **无** | 一次一条命令；无 MULTI/管道；现状路由统一要求 `database:write`，M1 改为读命令 read、写命令 write（§6.1 G3） |
| 对象存储 | `GET/POST/DELETE /s3/objects`、`GET /s3/presign` | JS `storage`、Go `UploadObject/ListObjects/DeleteObject/PresignObject` | 单文件约 50 MiB；列表上限 1000；预签名 15 分钟；只有私有对象 |
| 云函数 | `/gofunctions[/:name[/versions[/:ver[/activate|/test]]]]`；调用 `POST /go/:projectID/:name/:fn` | **无** | 单文件 `package main`、≤256 KiB、只能用已注册标准库、单入参单返回、10s；`activate` 省略时默认 true |
| 定时任务 | `/cron-jobs[/:id[/runs|/trigger]]` | **无** | `cron`（UTC 5 段）/ `interval` / `once(run_at)`；`input_json` 固定字符串 ≤8KB；结果截断 4KB；失败不重试；runs 无游标分页 |
| LLM | `/llm/chat`、`/llm/stream`、`/llm/providers`、`/llm/sessions*` | **无** | 需配置 provider；`llm.enabled=false` 时不挂载 |
| Cloud Agent | `/agents*`、`/agent-threads*`、`/agent-runs/:id/cancel`、`/agent-schedules*` | **无** | 项目内只读工具（database/s3/logs）；sandbox 无 SimpleBase 凭据 |
| 可观测 | `/logs`、`/metrics/summary`、`/metrics/trend`、`/audit`、`/quota`、`/settings` | **无** | 日志只记写请求 / 错误 / 慢请求 |

> SDK 缺口：JS 只能通过 `sb.raw.request` 调用上面标“无”的接口；Go SDK 没有导出通用请求方法，案例要自己写一个标准库 HTTP 小适配器（见 §6 G4）。

## 3. 场景设计与功能覆盖矩阵

### 3.1 四个场景（一个场景一个项目）

| 目录 | 项目 ID / 名称 | 后端形态 | 一句话 | 主要验证的开发习惯 |
|---|---|---|---|---|
| `examples/shop` | `ex-shop1` / 示例·电商 | Node BFF + JS SDK | 商品浏览、购物车、下单、日报 | 典型 CRUD + 事务 + 缓存；BFF 实时调用云函数算价 |
| `examples/community` | `ex-cmty1` / 示例·社区 | Go 服务 + Go SDK | 发帖带图、点赞、热榜、AI 摘要 | 文档模型 + SQL 混用；用户上传文件；排行榜 |
| `examples/iot-telemetry` | `ex-iot01` / 示例·物联网 | 无 BFF：`curl` / SDK 脚本 + 控制台 | 设备上报、分钟汇总、导出报表 | “只用控制台和 HTTP”的上手路径；批量写入；interval / once 定时；worker 消费运行记录 |
| `examples/ops-assistant` | `ex-ops01` / 示例·运维助理 | Go 服务 + Go SDK | 工单 + SLA 巡检 + Cloud Agent 日报 | 权限最小化、函数版本发布与回滚、Agent 定时、日志/审计/配额 |

项目 ID 固定，必须恰好 8 位，这样 init 能幂等地找到同一个项目；`--project-prefix` 只用于同一实例部署多套时改名（见 §5.4）。

### 3.2 覆盖矩阵（● 主验证，○ 辅助使用）

| 功能 | shop | community | iot | ops | 验证点 |
|---|---|---|---|---|---|
| 超管登录 + 创建项目 | ● | ● | ● | ● | init 用超管账号登录建项目，幂等 |
| 项目隔离（G0 修复后） | ● | ● | ● | ● | A 项目 Key 访问 B 项目任意资源均 403 |
| API Key 签发 / 吊销 / 最小权限 | ○ | ○ | ○ | ● | 服务 Key（read+write）；ops 额外签只读 Key：读 SQL / KV 读命令 / `/go` 调用放行，写操作 403；重复 init 时吊销旧 Key |
| 用户库创建 / 列表 | ● | ● | ● | ● | 每项目 1 个主库；community 另建 `media` 库，验证多库 |
| SQL query / execute | ● | ● | ● | ● | 参数化、分页、聚合 |
| SQL batch 事务 | ● | ○ | ● | ● | 下单扣库存；批量写遥测；工单状态和事件一起写 |
| 应用层唯一性 | ● | ● | ● | ○ | DDL 不写 PK / UNIQUE；用 UUID 主键 + `MERGE INTO` / KV `SET NX` + 事务内先查后写保证唯一（§6 G6） |
| 文档集合 CRUD | | ● | | ○ | 帖子正文存文档，点赞存 SQL；ops 工单附加字段存文档 |
| KV String / INCR / TTL | ● | ○ | ● | ● | 库存预占、设备在线心跳（EX）、幂等键 |
| KV Hash | ● | | ○ | | 购物车 HSET + EXPIRE |
| KV List | | | ● | ○ | 设备最近事件 LPUSH + LTRIM |
| KV Set | | ● | | ○ | 用户已点赞集合 SADD / SISMEMBER |
| KV ZSet | ○ | ● | | | 热榜 ZINCRBY / ZRANGE REV WITHSCORES |
| KV SCAN | | | ● | | 列出在线设备 |
| 对象上传 / 列表 | ● | ● | ● | ○ | 商品图、帖子附件、导出 CSV、Agent 日报 |
| 预签名下载 | ● | ● | ● | | 浏览器拿 15 分钟链接，过期后验证失败 |
| 对象删除 | | ● | ● | | 删帖时删附件；清理过期导出 |
| 云函数实时调用 `/go` | ● | ● | | ● | 算价 / 内容审核 / SLA 判定 |
| 云函数版本：创建 → test → activate | ● | ● | ● | ● | `activate:false` 先试跑再激活 |
| 云函数回滚到旧版本 | | | | ● | 发布一个故意错误的 v2 → test 失败 → 保留 v1；再演示手动 activate 回 v1 |
| 定时任务 cron 模式 | ● | | | ● | 日报 `0 1 * * *`、SLA 每小时 |
| 定时任务 interval 模式 | | ● | ● | | 热榜每 10 分钟；遥测汇总每 5 分钟 |
| 定时任务 once 模式 | | | ● | | 一次性导出报表（`run_at`） |
| 手动 trigger + runs | ● | ● | ● | ● | init 每个任务触发一次，控制台能看到 completed 记录 |
| worker 消费 runs 回写 | ○ | ○ | ● | ○ | 只在 iot 完整演示；其他场景只看结果（§6 G1） |
| LLM chat / sessions | | ● | | ○ | 帖子摘要；未配置 provider 时跳过并标“未验证” |
| Cloud Agent + agent-schedules | | | | ● | 每日只读巡检 SQL / 日志，输出到会话 |
| 日志 / 指标 / 审计 / 配额 | | | ○ | ● | ops 的“系统健康”页读 `/logs`、`/metrics/summary`、`/audit`、`/quota` |
| 项目设置 | | | | ○ | 读 `/settings`、调整日志保留 `/logs/retention` |

结论：4 个场景合起来覆盖 §2 中全部对外能力；每个场景至少 ● 验证 5 类功能，不存在只为凑数的场景。

## 4. 场景规格

### 4.1 shop — 电商（JS SDK）

- **数据（库 `shop`）**：`products(id, sku, name, price_minor, stock, image_key)`、`orders(id, user_id, status, total_minor, idem_key, created_at)`、`order_items`、`daily_stats(day, orders, gmv_minor, source_run_id)`。种子 20 个商品。`id` 统一用 UUID，`daily_stats` 按 `day` 用 `MERGE INTO` 写入。
- **KV**：`cart:<session>` Hash（30 分钟 TTL）；`stock:hold:<sku>` 用 `INCRBY` 预占，下单成功或过期后释放；`cache:hot` String 缓存热销 JSON。
- **对象**：`products/<sku>.webp` 商品图（init 上传 `seed/images/`）；BFF 为列表页批量签出预签名 URL，有效 15 分钟，前端过期后自动刷新。
- **云函数 `pricing.go`**：`Quote({items, coupon})` 返回明细和折扣，BFF 下单前实时调用；`Tick({})` 返回前一 UTC 自然日窗口。
- **定时**：`shop_daily` cron `0 1 * * *` → `pricing.Tick`；BFF 带 `EXAMPLE_WORKER=1` 启动时消费 run，`MERGE INTO daily_stats` 回写。
- **BFF API**：`GET /api/products`、`PUT /api/cart`、`POST /api/orders`、`GET /api/orders/:id`、`GET /api/stats`。
- **验收**：重复提交同一 `Idempotency-Key` 只生成一单（KV `SET NX EX` 抢占 + 事务内 `SELECT … WHERE idem_key = ?` 复查）；库存不足时 batch 回滚；控制台 SQL 页能查到订单。

### 4.2 community — 社区（Go SDK）

- **数据**：主库 `community`，文档集合 `posts`（标题、正文、作者、附件 key）；SQL `likes(post_id, user_id, created_at)`、`post_stats(post_id, likes, score)`。第二个库 `media` 存附件元数据 `attachments(key, post_id, size, mime)`，用来验证多库和 SDK 的 `Database(id)` 切换。
- **KV**：`liked:<user>` Set 防重复点赞；`rank:hot` ZSet 热榜；`post:<id>` String 缓存详情（5 分钟 EX）。
- **对象**：`posts/<post_id>/<uuid>.<ext>`，服务端校验 MIME 和大小（≤5 MiB）后通过 SDK 上传；删帖时删除对象和文档，再清 KV。
- **云函数 `moderation.go`**：`Check({title, body})` 做关键词和长度规则，发帖前实时调用；`Score({items})` 计算热度。
- **定时**：`community_hot` interval 600s → `moderation.Score`，`input_json` 固定为空窗口；服务端消费 run，重建 `rank:hot`。
- **LLM**：`POST /api/posts/:id/summary` 调 `/llm/chat` 生成摘要，存回文档；`GET /llm/providers` 为空时接口返回 501，并在 README 标注。
- **验收**：同一用户重复点赞计数不变；文档写成功但 SQL 失败时有补偿（删除文档）并有测试覆盖；删帖后附件列表为空。

### 4.3 iot-telemetry — 物联网（无 BFF）

定位：演示“不写后端，只用 HTTP / 控制台”的路径，也是 runs 消费模式的标准实现。

- **数据（库 `telemetry`）**：`readings(device_id, ts, metric, value)`、`rollup_5m(device_id, bucket, metric, avg, max, count, source_run_id)`、`devices(id, name, model)`。种子 10 台设备、2 小时模拟数据（init 用 batch 写入，每批 ≤500 行）。
- **KV**：`online:<device>` String `EX 120` 心跳；`events:<device>` List `LPUSH` + `LTRIM 0 99`；`SCAN MATCH online:*` 列出在线设备；`counter:readings` `INCRBY`。
- **对象**：`exports/<date>/readings.csv` 由导出脚本生成并上传，返回预签名链接；`cleanup` 脚本删除 7 天前的导出。
- **云函数 `rollup.go`**：`Window({})` 返回最近一个已完成的 5 分钟窗口；`Aggregate({points})` 算 avg/max/count。
- **定时**：`iot_rollup` interval 300s → `rollup.Window`；`iot_export_once` once（`run_at` = init 时间 + 10 分钟）→ `rollup.Window`，演示一次性任务执行后自动停止。
- **脚本（`scripts/*.mjs` 与等价 `curl.sh`）**：`simulate.mjs` 持续上报、`worker.mjs` 消费 runs（`job_id+run_id` 去重写 `rollup_5m`，积压 100 条或 4KB 截断时停止并告警）、`export.mjs`、`cleanup.mjs`。
- **验收**：不跑任何脚本时控制台就能看到种子数据、KV、导出文件和 runs；跑 `worker.mjs` 后 `rollup_5m` 增长，重启 worker 不重复写入。

### 4.4 ops-assistant — 运维助理（Go SDK）

- **数据（库 `ops`）**：`tickets(id, title, priority, status, due_at, version)`、`ticket_events`、`sla_marks(ticket_id, level, run_id)`；工单附加字段存文档集合 `ticket_meta`。
- **KV**：`idem:<key>` `SET NX EX 600`；`queue:pending` List 作为待办缓存。
- **权限**：init 为本项目签两把 Key：`service`（database:read/write）和 `readonly`（database:read）。`scripts/verify-perms.sh` 用只读 Key 测试：
  - 必须放行：SQL query、KV `GET` / `HGETALL` / `ZRANGE` / `SCAN`、`POST /go/...` 调用、`GET /s3/objects`；
  - 必须 403：SQL execute、KV `SET` / typed 写入、`POST /gofunctions`、`POST /s3/objects`、`POST /cron-jobs`；
  - 用本项目 Key 访问 `ex-shop1` 的任意资源必须 403（G0）。
- **云函数 `sla.go`**：`Evaluate({tickets})` 返回 SLA 等级。发布流程演示 v1 激活 → 上传故意编译失败 / test 失败的 v2（`activate:false`），确认 v1 仍在服务 → 修好后发布 v3 → `activate` v1 回滚。
- **定时**：`ops_sla_scan` cron `0 * * * *` → `sla.Evaluate`（输入固定为空列表，演示 §6 G2 的局限）；服务端 worker 读真实工单后自己调用 `/go`。
- **Cloud Agent**：init 创建 `ops-daily` agent（只读工具：database / logs），`agent-schedules` 每天 `0 2 * * *` 运行，提示词为“汇总昨日新增工单与 5xx 日志”；没有 LLM provider 时只创建、不启用。
- **系统健康页**：`/logs?level=…`、`/metrics/summary`、`/metrics/trend`、`/audit`、`/quota` 聚合展示；`/logs/retention` 读写演示（需 project:admin，只在 init 阶段用超管 token 设置）。
- **验收**：`verify-perms.sh` 全部符合预期；回滚演示后 `/go` 返回 v1 结果；agent 和 schedule 在控制台可见，有 provider 时能手动触发一次 run。

## 5. 目录结构与 init.sh

### 5.1 统一目录约定

```text
examples/
  README.md                    # 总览：场景、覆盖矩阵、init 用法、缺口链接
  init.sh                      # 唯一入口（bash + curl + node，零新依赖）
  lib/
    init/*.mjs                 # init 用的 Node 步骤脚本（清单解析、幂等 upsert）
    node/sb.mjs                # JS：createClient + kv()/fn()/cron() 薄封装（基于 sb.raw.request）
    goexample/                 # Go：HTTP 小适配器（KV / 函数 / cron / LLM / agent）+ 共享 worker
  <case>/
    simplebase.json            # 场景清单：项目、库、迁移、种子、KV、对象、函数、任务、agent、Key
    migrations/001_init.sql    # 只用 DuckLake 支持的 DDL：不写 PRIMARY KEY / UNIQUE / 索引 / 序列
    seed/*.sql | *.json | images/
    functions/<name>.go        # 单文件 package main
    server/ 或 scripts/        # 案例代码（启动时不再建表）
    web/                       # 静态页，只请求同源 BFF
    README.md                  # 运行、验证清单、限制
    .env                       # init 生成，600，已被 .gitignore 忽略
```

`simplebase.json` 示例（字段即 init 的输入，init 不写死任何场景逻辑）：

```json
{
  "project": { "id": "ex-shop1", "name": "示例·电商" },
  "databases": [{ "name": "shop", "migrations": "migrations", "seed": "seed/shop.sql" }],
  "kv": "seed/kv.json",
  "objects": [{ "dir": "seed/images", "prefix": "products/" }],
  "functions": [{ "name": "pricing", "file": "functions/pricing.go", "test": { "function_name": "Quote", "body": { "items": [] } } }],
  "cron_jobs": [{ "name": "shop_daily", "schedule_kind": "cron", "cron_expr": "0 1 * * *", "func_file": "pricing", "func_export": "Tick", "input_json": "{}", "trigger_once": true }],
  "api_keys": [{ "label": "service", "permissions": ["database:read", "database:write"] }],
  "requires": { "llm": false }
}
```

### 5.2 init.sh 流程

```bash
./examples/init.sh [--url URL] [--user U --password P] [--only shop,iot-telemetry]
                   [--project-prefix ex] [--skip-build] [--reset-data] [--dry-run]
```

认证只有一种方式：**超管账号登录**。默认用种子超管 `simplebase2026`，可通过 `--user/--password` 或环境变量 `SIMPLEBASE_ADMIN_USER` / `SIMPLEBASE_ADMIN_PASSWORD` 覆盖。不接受 `--api-key`，不使用 DevMode 种子 Key `sb_live_dev_key_12345`。登录后检查返回的 `user.role` 必须是 `superadminl1`，否则退出（`admin` 角色只读，不能建项目；`user` 角色看不到其他人的项目）。access token 只保存在进程内存；init 运行超过 1.5 小时时用 refresh_token 续期。

| 步骤 | 动作 | 幂等规则 |
|---|---|---|
| 0 前置 | `GET /health/ready`；需要 `curl`、`node`（Go 场景另需 `go`）；首次构建 `packages/js-sdk` | 失败立即退出，提示 `./build.sh dev` |
| 1 登录 | `POST /v1/auth/login`（默认种子超管，可用环境变量 `SIMPLEBASE_ADMIN_USER/PASSWORD` 或 `--token` 覆盖），拿 JWT | JWT 只在内存中用，不落盘 |
| 2 项目 | `GET /v1/projects` 有同 ID 则复用，否则 `POST /v1/projects {id,name}` | 不用 DevMode 种子 Key：它只绑定 `dev-shop`，不能在新项目里签 Key（§6 G5） |
| 3 数据库 | 按名称查找，没有就创建；记录 DB ID | 409 时回查 |
| 4 迁移 | 按文件名顺序执行 `migrations/*.sql`；用 `_migrations(name, applied_at)` 表记录（`MERGE INTO … ON name` 写入）；执行前扫描 DDL，出现 `PRIMARY KEY` / `UNIQUE` / `CREATE INDEX` / `CREATE SEQUENCE` 直接报错（G6） | 已应用的跳过；不支持 down（§6 G9） |
| 5 种子 | 执行 `seed/*.sql`，仅当 `_migrations` 里没有 `seed:<file>` 标记时执行；`--reset-data` 会先 `DELETE` 案例表再重灌 | 不自动删库删表；数据乱了就整实例重置（§5.4） |
| 6 KV | 按 `seed/kv.json` 逐条下发命令；带 TTL 的 key 每次都重写 | 一次一条 HTTP（§6 G8） |
| 7 对象 | 上传 `objects[].dir` 下文件，自动推断 MIME；同 key 覆盖 | 不删除清单外的对象 |
| 8 云函数 | 没有函数就 `POST /gofunctions {activate:false}`，源码有变化就 `POST .../versions {activate:false}` → `POST .../:ver/test` → `ok` 才 `activate` | 源码一致（比较 sha256）则跳过；test 失败保留旧激活版本并标记失败 |
| 9 定时任务 | `GET /cron-jobs` 按 name 匹配，没有就 POST，配置变化就 PATCH；`trigger_once` 时 `POST trigger` 并轮询 runs 至 completed/failed | 不重复创建 |
| 10 agent | 仅 `requires.llm` 满足且 `GET /llm/providers` 非空时创建 agent 和 agent-schedule；否则跳过并标“未验证” | 按 name 匹配 |
| 11 Key | 用超管 JWT 为每个 `api_keys[]` 签发**运行时 Key**（最小权限，供 BFF / 脚本使用），写入 `.env`；吊销 `.env` 里记录的旧 Key ID | 每次 init 都会轮换 Key（Key 没有标签，无法按名复用） |
| 12 构建 | `npm run build` / `go build ./examples/<case>/...`；`--skip-build` 跳过 | 失败只标记该场景 |
| 13 自检 | 每个场景执行 `smoke`：SQL 计数、KV GET、对象列表、`/go` 调用、runs 最新状态；另外做一次**跨项目负向测试**（用 A 项目 Key 访问 B 项目的 KV / S3 / 函数，期望 403，§6 G0） | 输出汇总表 |

结束时打印每个场景的控制台入口（`http://127.0.0.1:5173/?project=<id>`）、启动命令和“控制台验证清单”。

### 5.3 控制台即可验证（不启动案例代码）

| 控制台页面 | 打开 `ex-shop1` 后应看到 |
|---|---|
| 数据库 → `shop` | 4 张业务表 + `_migrations`；`SELECT * FROM products` 返回 20 行 |
| Key-Value | `cart:demo`（Hash，带 TTL）、`cache:hot` |
| 文件存储 | `products/*.webp` 共 20 个，可预览、可生成链接 |
| 云函数 | `pricing.go` v1 已激活，导出 `Quote` / `Tick`，可在线试跑 |
| 定时任务 | `shop_daily` 已启用，运行记录 1 条 completed |
| 设置 → 连接 | 1 把 `service` Key |

其余三个项目各有同样的清单，写在各自 README 的“验证清单”一节。

### 5.4 多套部署与清理

- `--project-prefix t1` 会把 `ex-shop1` 改成 `t1-shop0`（前缀 + 场景短码，补足或截断到 8 位），用于同一实例部署多套；生成的 ID 不能与 `dev-shop`、`sb-admin` 冲突。
- **出问题就整实例重置**（G7 接受现状，不做删除项目）：

  ```bash
  ./build.sh reset -y      # 删除本地 cache_dir（不动远程 S3），清空全部项目和数据
  ./build.sh dev           # 重新生成种子超管等系统数据
  ./examples/init.sh       # 重新创建 4 个场景项目
  ```

  如果配置了远程 S3 且需要连远端一起清空，用 `./simplebased reset --confirm=<instance_id>`（不加 `--local-only`）。轻量场景可以只用 `./examples/init.sh --reset-data` 清空案例表数据。README 首页把“整实例重置”写成标准恢复手段，并提示：**会清掉同一实例上的所有其他项目**。

## 6. 平台缺口与处理决策

### 6.1 本轮服务端修复（M1，先于案例开发）

#### G0 项目归属校验（P0 安全）

**问题**：`auth.Require` 只检查权限位（注释写明归属由 handler 负责）；`projectContextMiddlewareEcho` 只调用 `ResolveProjectTenant` 解析租户。KV、S3、gofunctions、cron-jobs、`/go`、llm、agents、logs、metrics、settings、audit、quota 等 handler 直接使用 `pc.ID`，没有调用 `CanAccessProject`。只有 databases / SQL / data 经 catalog `ensureProjectAccess` 做了校验，api-keys handler 自己做了校验。结果是：持有 A 项目 Key 的人可能读写 B 项目的 KV、对象、云函数和定时任务。

**修复方案**：

1. 在 `projectContextMiddlewareEcho`（`internal/api/database_handler.go`）解析出 tenant 后，统一做归属判断，规则与 catalog `ensureProjectAccess` 一致：
   - 从 ctx 取 Principal，没有则 401；
   - `principal.TenantID != tenantID` → 403 `cross_project_denied`；
   - 系统项目 `sb-admin`：只放行 super / admin 角色或持 `project:admin` 的 Key；
   - 登录态 user 角色（**先判断角色**：即使它带有 `project:admin` 权限位也不能放宽）、普通 API Key：必须 `CanAccessProject(projectID)`；
   - super / admin 角色，以及持 `project:admin` 的 Key：同租户放行（保持现有语义）。
2. 判断逻辑抽成 `auth` 包里的纯函数（如 `auth.CheckProjectAccess(p, projectID, tenantID, isSystem)`），catalog `ensureProjectAccess` 也改为调用它，两处规则只保留一份。
3. `/v1` 组和 `/go/:projectID` 组都会经过这个中间件，所以一处修改覆盖全部带 `:projectID` 的路由；api-keys handler 里原有的检查保留，作为第二道防线。
4. 中间件执行顺序不变：auth → project context → Require → handler。

**需要保留的现有行为**：持 `project:admin` 的 Key 可以访问同租户其他项目（catalog 目前就这样设计，DevMode 种子 Key 依赖这一点）。这项规则不收紧，只在 §6.2 记为后续讨论项 G0b。

**测试**（`internal/api` 中挂 fake catalog，按 AGENTS.md 要求不依赖真实 DuckDB）：
- 表驱动用例：{普通 Key（本项目 / 他项目）、`project:admin` Key、user 角色（own / 他人）、admin 角色、super 角色} × {kv、s3/objects、gofunctions、cron-jobs、`/go`、llm/providers、agents、logs、databases}，覆盖期望的 200 / 403；
- 系统项目 `sb-admin`：普通 Key 403；
- 租户不一致：403；
- 回归：现有 `go test ./internal/...` 全部通过。

#### G3 KV 读写权限拆分（P1）

**问题**：`router.go` 中 `p.POST("/kv", kvh.Execute, require(auth.DatabaseWrite))` 让读命令也需要 write 权限；但 handler 已经按 `kvCommandTable[name].writable` 区分 `ReadOnly` / `ReadWrite`，文档 `docs/database/kv.md` 也写的是“读命令 read，写命令 write”。

**修复方案**：

1. 路由改为 `require(auth.DatabaseRead)`。
2. `KVHandler` 在 `execCmd` 拿到 `spec.writable` 后，以及 `execTyped`（typed 写入一律视为写）中，在 `acquireStore` 之前检查 `principal.HasPermission(auth.DatabaseWrite)`，没有则返回 403 `forbidden`。
3. 未知命令仍然先返回 400 `kv_unknown_command`（不泄露权限信息的前提下保持现有错误优先级）。
4. 文档不用改（已是目标语义）；在 `docs/database/kv.md` 的“请求”一节补一句“只读 Key 可执行读命令”。

**测试**：read-only principal 执行 `GET` / `HGETALL` / `SCAN` / `ZRANGE` → 200；`SET` / `HSET` / `DEL` / typed `String` → 403；write principal 行为不变；系统项目仍返回 404 `kv_not_found`。

### 6.2 其余缺口（本轮不改服务端，只记录）

严重度：**P0** 安全 / 正确性；**P1** 阻碍常见开发习惯；**P2** 体验。

| ID | 缺口 | 现状（代码依据） | 案例怎么绕 | 建议 / 决策 |
|---|---|---|---|---|
| G0 | 项目归属校验缺失 | 见 §6.1 | — | **本轮修复（M1）** |
| G0b | `project:admin` Key 可以跨项目访问同租户资源 | catalog `ensureProjectAccess` 的现有设计 | 案例运行时 Key 不授予 `project:admin` | 后续评估是否收紧为仅本项目 |
| G1 | 云函数不能读写数据库 / KV / S3 | 解释器只注册标准库；没有注入凭据的机制 | 函数只做纯计算，可信服务消费 runs 后回写 | 注册受限的宿主包（如 `simplebase/db`），按项目作用域注入 |
| G2 | cron 入参固定，不能动态取数 | `input_json` 创建时固定 | `Tick` 只返回时间窗，worker 查库后调 `Compute` | 同 G1；或支持“任务结果 webhook” |
| G3 | KV 读命令也要 `database:write` | 见 §6.1 | — | **本轮修复（M1）** |
| G4 | SDK 不覆盖 KV / 云函数 / cron / LLM / agent / 日志；Go SDK 也没有通用请求方法 | JS 只有 `raw.request`；Go `request` 未导出 | `examples/lib` 提供薄封装 | SDK 补 `kv` / `functions` / `cron` 模块，Go 导出 `Do` |
| G5 | 种子 Key 不能给新项目签 Key；Key 没有标签和过期时间 | 种子 Key 只绑定 `dev-shop`；明文只出现一次 | **决策：init 只用超管账号登录**（§5.2），管理操作全部用超管 JWT；运行时 Key 每次 init 轮换 | Key 的 `label` / `expires_at` 列入后续 |
| G6 | 不支持 PRIMARY KEY / UNIQUE / 索引 / 序列 | DuckLake 规范不支持，声明了也**不保证唯一** | **决策：案例 DDL 去掉这些声明**，唯一性由应用层保证：UUID 主键、`MERGE INTO … ON <业务键>`、KV `SET NX`、事务内先查后写（依赖单写实例）；init 迁移检查拒绝这些关键字 | `docs/database/index.md` 补一节“约束与唯一性”；sqlguard 告警列入后续 |
| G7 | 不能删除项目 | 没有 `DELETE /v1/projects/:id` | **决策：接受现状**，出问题时整实例重置（§5.4） | 本轮不做 |
| G8 | KV 没有批量 / 事务 / 管道 | 一条 HTTP 只执行一条命令 | 种子按条下发；优先用 `MSET` / `HSET` 多字段合并 | 支持 `{"type":"pipeline"}` |
| G9 | 没有迁移工具 | 只能执行 SQL | init 用 `_migrations` 表实现 up-only 迁移 | 提供迁移 API / CLI |
| G10 | 没有公开静态托管 / CDN / CORS | 对象私有，预签名 15 分钟；没有 CORS 中间件 | 前端由 BFF 同源提供；不把对象当网站 | 公开 bucket 前缀 + 静态托管；可配置 CORS |
| G11 | runs 无游标分页，结果截断 4KB | `GET runs?limit=100` | worker 积压或截断时停止并告警，人工补算 | 加 `cursor` / `since`；结果改存对象 |
| G12 | cron 失败不重试，也没有告警 | docs/cronjob 第 6 条 | ops 健康页展示最近失败的 run | 可配重试次数；失败 webhook |
| G13 | 浏览器端没有终端用户认证 | Key 不能下发到浏览器；没有 RLS / 匿名 Key | 全部经 BFF；演示时用固定 demo 用户 | 设计 anon Key + 行级策略（对标 Supabase） |
| G14 | 没有 Realtime / 订阅 | — | 前端轮询 | 列入长期规划 |
| G15 | 文档与 SQL 不能在同一事务 | 两套接口 | community 用补偿删除 | 文档 API 支持加入 batch |
| G16 | 函数日志不可见 | `print` 写入 Context，`/go` 不返回 | 函数把调试信息放进返回值 | test 接口返回 `Output()` |

实施时每个场景 README 的“限制”一节只引用 G 编号，不再重复描述。

## 7. 迁移与实施里程碑

| 阶段 | 内容 | 退出条件 |
|---|---|---|
| M0 盘点 | 列出旧 `examples/*` 被 docs、`build.sh`、测试引用的位置；旧 5 案例映射到新场景（shop+booking → shop；community+mini-game → community；ticket → ops） | 引用清单完成；`git status` 干净 |
| M1 平台修复 | 按 §6.1 修复 G0（项目归属中间件 + 抽出 `auth.CheckProjectAccess`）和 G3（KV 路由改 read + handler 内写命令鉴权）；补表驱动测试；`docs/database/kv.md` 补一句说明；`docs/database/index.md` 补“约束与唯一性”（G6） | `go build ./internal/... ./cmd/...`、`go test ./...` 通过；新增的跨项目、KV 只读用例全绿；**M1 合入前不开始 M2** |
| M2 init 框架 | `lib/init/*` + `simplebase.json` 解析，先只接入 iot（无 BFF，最简单） | iot 在空实例上 init 两次结果一致；§5.3 清单全部通过 |
| M3 shop / community | 迁移旧代码，去掉启动时建表和 `PRIMARY KEY` 声明，改为读迁移结果；唯一性改走应用层 | 单测 + smoke 通过；控制台清单通过；重复下单 / 点赞测试通过 |
| M4 ops | Key 权限、函数回滚、agent、可观测页 | `verify-perms.sh` 全部符合 §4.4 预期；无 LLM 时 agent 被标记为跳过 |
| M5 清理 | 删除旧 `booking-service` / `ticket-service` / `mini-game-service` / `_template` / `shared`（`git rm`，不用 `rm -rf`）；更新 `examples/README.md`、`docs/sdk/examples.md`、`plan/planv4.0/cleanup-plan.md` §E | 不留旧路径引用；回归命令全部通过 |

## 8. 验收

1. 在空实例（`./build.sh reset -y && ./build.sh dev`）上执行 `./examples/init.sh`（只用超管账号登录）：4 个项目创建成功，§5.3 和各 README 的控制台清单全部可见；再执行一次没有新增重复资源（Key 轮换除外）；跨项目负向测试全部 403。
2. 项目隔离与权限：§6.1 的 G0 / G3 测试纳入 `go test ./internal/api/...`，并保持通过。
3. 案例 `migrations/` 中 grep 不到 `PRIMARY KEY`、`UNIQUE`、`CREATE INDEX`、`CREATE SEQUENCE`。
4. 每个场景：`npm test` / `go test ./examples/<case>/...` 通过；按 README 启动后，主业务流程可以在浏览器或脚本中走通。
5. §3.2 矩阵每个 ● 至少对应一条自动化 smoke 或测试；LLM / agent 在没有 provider 时输出“未验证”，不能算通过。
6. §6.2 每条缺口都有对应的复现命令或测试，并在 README 中引用。
7. 回归：`go build ./internal/... ./cmd/...`、`go test ./...`、`go test ./gofunction/... -short`、`go test ./packages/go-sdk`、`cd packages/js-sdk && npm run typecheck && npm test`。
8. 安全：仓库不提交 `.env`、Key 和 JWT；init 不读取、不使用 `sb_live_dev_key_12345`；前端 bundle 中 grep 不到 `sb_live_`；init 日志不打印密钥和 token。
