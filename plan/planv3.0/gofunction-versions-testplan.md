# SimpleBase v3.0 云函数版本 + 调试台（Test Bench）设计

> **状态**：**已实施完成**（逐项执行状态见 §14；P1–P5 全部落地，验收 A1–A12 见 §10 状态列）  
> **日期**：2026-09-24  
> **范围**：云函数版本模型 / 列表「测试」入口 / Postman 风格调试 Modal / 管理面与 `/go` 调用面重构  
> **前提**：**新项目，不兼容旧行为**——`sys_gofunctions` 与现有 CRUD/调用语义可整体重设计  
> **关联**：[`login-auth-plan.md`](./login-auth-plan.md)、[`ui-gofunction-plan.md`](../planv2.0/ui-gofunction-plan.md)、[`ui-principles.md`](../planv2.0/ui-principles.md)、`ui/AGENTS.md`、`internal/AGENTS.md`

---

## 0. 一句话目标

**云函数引入「多版本 + 指定生效版本」；列表增加「测试」；测试台 Modal 左栏选导出函数/版本，右栏 Postman 式填 URL 与 JSON、看状态码与响应体。**

---

## 1. 背景与现状（已核实）

| 层 | 现状 | 问题 |
|---|---|---|
| 存储 | `sys_gofunctions(id, project_id, name, source, exports_json, …)`，**一文件一源码**，`Update` 直接覆盖 | **无版本**；覆盖后无法回滚/灰度；无法「哪一版生效」 |
| 导出 | 保存时 `ParseFuncList` 写入 `exports_json` | 导出列表与源码绑定，但**没有按版本快照** |
| 管理 API | `GET/POST /gofunctions`，`GET/PUT/DELETE /gofunctions/:name` | 无 versions、无 activate、无 test |
| 调用 API | `POST /go/:projectID/:name/:functionName`，总是跑当前 `source` | 无法指定版本；无法安全试跑未生效版 |
| UI 列表 | 操作：查看 / 编辑 / 复制路径 / 删除 | **无「测试」** |
| UI 弹窗 | `GoFunctionModal`（Monaco 改源码） | **无调试台**；只能复制 URL 外部工具测 |
| Modal 能力 | 已有公共 `SbModal`（`maxWidth` / `hideFooter` / 自定义 footer） | 适合承载 Postman 布局 |

---

## 2. 决策锁定

| # | 决策 | 说明 | 执行 |
|---|---|---|---|
| D1 | **重设计，不兼容** | 允许丢弃旧 `sys_gofunctions` 数据；迁移直接建新表（v34+），不再读旧 schema | ✅ |
| D2 | **函数实体 + 版本快照分离** | `sys_go_funcs` 管「名字 / 生效版本指针」；`sys_go_func_versions` 管「不可变源码快照 + exports」 | ✅ |
| D3 | **版本号单调递增** | 同 `func_id` 下 `version = 1,2,3…`；版本行**创建后不可改源码**（新编辑 = 新版本）；每函数版本数上限 **50** | ✅ |
| D4 | **唯一生效版（active）** | 每个函数恰好一个 `active_version`；**所有生产调用只跑生效版** | ✅ |
| D5 | **测试可打任意版本** | 调试台可试跑历史版/未发布新版本，**不影响** `active_version` | ✅ |
| D6 | **测试台 = Postman 布局 Modal** | 公共 `SbModal`（`maxWidth≈960`，`hideFooter`）；**仅左栏**含版本+导出函数（满足「左侧要有版本」），右栏 URL/Body/响应 | ✅ |
| D7 | **管理面测试走专用端点** | `POST …/gofunctions/:name/versions/:ver/test`，登录态 + `database:write`；与 `/go` 对外调用隔离 | ✅ |
| D8 | **生产调用只认生效版** | `/go`、**定时任务**、其他内部 Runner **全部**解析 `active_version` 后取该版 source；`?version=` 默认拒绝 | ✅ |
| D9 | **exports 按版本快照** | 每个版本行保存该版 `exports_json`；左栏按**所选版本**展示 | ✅ |
| D10 | **列表列增加版本信息** | 见 §6.1：生效版本、最新版本、导出、更新时间、操作 | ✅ |
| D11 | **展示 URL ≠ 测试通道** | 右栏只读 URL 是对外 `/go/...`（调用方自带 API Key）；调试台**实际发送**管理端 test API（浏览器带登录态），避免前端暴露/伪造 Key | ✅ |
| D12 | **无生效版时列表 exports** | `exports` = 生效版导出；`active_version=0` 时取**最新版**导出，且 UI 标注「未发布」 | ✅ |

### 2.1 生产调用统一语义（重要）

```text
                    ┌── POST /go/:pid/:name/:fn     （外部 SDK / curl）
解析 active_version ─┼── sys_cron_jobs（func_file+func_export）  （定时任务 Runner）
                    └── 未来 Agent 工具 / 云函数互调
                              │
                              ▼
                    sys_go_func_versions.source WHERE version = active
```

- 定时任务**不**记录 version 钉死到 job（避免回滚后仍跑旧版）；每次触发现查 `active_version`。
- Job 目标校验改为：`func` 存在 **且** `active_version>0` 且该版 `exports` 含 `func_export`。
- 无生效版时任务标记失败：`no_active_version`。

---

## 3. 产品模型

```text
云函数（逻辑文件） name = hello
  │
  ├─ v1  source_A  exports=[Hello, Ping]     ← 历史
  ├─ v2  source_B  exports=[Hello]           ← 历史
  └─ v3  source_C  exports=[Hello, Ping]     ← ★ active_version=3（线上生效）
         ▲
         │ 保存源码 → 创建 v4（草稿可直接 activate）
```

| 概念 | 标识 | 规则 |
|---|---|---|
| 云函数 | `name` | `^[A-Za-z][A-Za-z0-9_]{0,62}$`；项目内唯一（未归档） |
| 版本 | `version` | 正整数，项目+函数内唯一；不可变快照 |
| 生效版本 | `active_version` | 写在 `sys_gofunctions`；切换即发布/回滚 |
| 导出函数 | `exports[]` | 该**版本**源码解析结果；调用 URL 最后一段 |
| 测试调用 | test invoke | 指定 `(name, version, functionName, JSON body)`，返回状态码+耗时+JSON |

**明确不做（本期）**：版本 diff UI、灰度流量、定时发布、多人锁、函数级依赖管理。

---

## 4. 系统表设计（重设计，迁移 v34 起）

> DuckLake 无 PK；唯一性应用层 + 冲突重试。旧 `sys_gofunctions`（v28）**整表废弃**，新表用新名字避免歧义。

### 4.1 `sys_go_funcs`（v34）— 函数实体

```sql
CREATE TABLE IF NOT EXISTS sys_go_funcs (
    id              VARCHAR NOT NULL,          -- UUID
    project_id      VARCHAR NOT NULL,
    name            VARCHAR NOT NULL,          -- hello（不含 .go）
    active_version  BIGINT NOT NULL DEFAULT 0, -- 0 = 尚未发布；>0 = 生效版本号
    description     VARCHAR NOT NULL DEFAULT '',
    created_by      VARCHAR NOT NULL DEFAULT '',
    created_at      TIMESTAMP NOT NULL,
    updated_at      TIMESTAMP NOT NULL,
    archived_at     TIMESTAMP
)
```

### 4.2 `sys_go_func_versions`（v35）— 版本快照（不可变）

```sql
CREATE TABLE IF NOT EXISTS sys_go_func_versions (
    id           VARCHAR NOT NULL,             -- UUID
    func_id      VARCHAR NOT NULL,             -- → sys_go_funcs.id
    project_id   VARCHAR NOT NULL,
    name         VARCHAR NOT NULL,             -- 冗余，便于按名查询
    version      BIGINT NOT NULL,              -- 1,2,3…
    source       VARCHAR NOT NULL,             -- 完整 Go 源码（权威）
    exports_json VARCHAR NOT NULL,             -- ["Hello","Ping"]
    note         VARCHAR NOT NULL DEFAULT '',  -- 版本备注（发布说明）
    created_by   VARCHAR NOT NULL DEFAULT '',
    created_at   TIMESTAMP NOT NULL
)
```

唯一性：`(func_id, version)`、`(project_id, name, version)`。

**派生字段（不落列，查询时算）**

| 字段 | 算法 |
|---|---|
| `latest_version` | `MAX(version) FROM sys_go_func_versions WHERE func_id=?` |
| 列表 `exports` | 见 D12：active>0 取 active 版；否则取 latest 版并标 `published=false` |
| `source`（编辑器） | 默认 `GET …/versions/{active_version}`；无 active 则 `latest`；历史版只读打开 |

**version 分配**：`INSERT version = MAX+1`；若唯一冲突（并发）则重读 MAX 重试 ≤3 次。

### 4.3 `sys_go_func_invokes`（v36）— 可选：测试/调用审计摘要

```sql
CREATE TABLE IF NOT EXISTS sys_go_func_invokes (
    id            VARCHAR NOT NULL,
    project_id    VARCHAR NOT NULL,
    func_name     VARCHAR NOT NULL,
    function_name VARCHAR NOT NULL,
    version       BIGINT NOT NULL,
    channel       VARCHAR NOT NULL,            -- go | test
    status_code   BIGINT NOT NULL,
    duration_ms   BIGINT NOT NULL,
    request_id    VARCHAR NOT NULL,
    actor         VARCHAR NOT NULL DEFAULT '', -- user_id 或 api_key_id
    created_at    TIMESTAMP NOT NULL
)
```

> 不存请求/响应原文（脱敏与体积）；调试台前端展示响应即可。

### 4.4 关系

```text
sys_go_funcs 1 ──── n sys_go_func_versions
     │
     └── active_version ──► 某一 version（逻辑外键）
sys_go_func_invokes（旁路流水，可选）
```

---

## 5. 后端详细设计

### 5.1 包与职责

```text
internal/systemdb/
  gofuncs.go            # 实体 CRUD + 版本追加 + Activate
  gofunc_invokes.go     # 调用流水（可选）

internal/api/
  gofunction_handler.go # 重写：实体 + versions + test
  router.go             # 挂载新路由；/go 调用改读 active_version

gofunction/             # 不变：ParseFuncList / RunJSON
```

### 5.2 管理面 API（`/v1/projects/:projectID/gofunctions`，需登录态/Key）

| 方法 | 路径 | 权限 | 语义 | 执行 |
|---|---|---|---|---|
| GET | `/gofunctions` | `database:read` | 列出实体（含 `active_version` / `latest_version` / `exports` 取生效版） | ✅ |
| POST | `/gofunctions` | `database:write` | 创建实体 + **v1** 版本；可选 `activate=true`（默认 true） | ✅ |
| GET | `/gofunctions/:name` | `database:read` | 实体 + 版本摘要列表 | ✅ |
| PATCH | `/gofunctions/:name` | `database:write` | 改 `description`；**不改源码** | ✅ |
| DELETE | `/gofunctions/:name` | `database:write` | 软删实体 + 归档全部版本 | ✅ |
| GET | `/gofunctions/:name/versions` | `database:read` | 版本列表（不含 source，或 `?with_source=1`） | ✅（无 `?with_source`，source 详情走 GET ver） |
| POST | `/gofunctions/:name/versions` | `database:write` | **新建版本**（body: `source`, `note?`, `activate?`） | ✅ |
| GET | `/gofunctions/:name/versions/:ver` | `database:read` | 版本详情（含 source + exports） | ✅ |
| POST | `/gofunctions/:name/versions/:ver/activate` | `database:write` | **发布/回滚**到该版本 | ✅ |
| POST | `/gofunctions/:name/versions/:ver/test` | `database:write` | **调试台试跑**（见 5.3） | ✅ |

错误码：`gofunction_not_found` · `version_not_found` · `invalid_gofunction_name` · `invalid_exports` · `version_exists`。

### 5.3 测试调用（调试台专用）

```http
POST /v1/projects/{pid}/gofunctions/{name}/versions/{ver}/test
Authorization: Bearer <jwt | api-key>
Content-Type: application/json

{
  "function_name": "Hello",
  "body": { "name": "world" }
}
```

```jsonc
// 200（业务成功/失败都用 200 包一层，便于 UI 展示；业务错误在 payload）
{
  "ok": true,
  "status_code": 200,
  "duration_ms": 12,
  "version": 3,
  "active_version": 3,
  "function_name": "Hello",
  "data": { "message": "hi" },
  "error": ""
}

// 400 function_name 非法 / body 非 JSON
// 404 函数或版本不存在
// 200 + ok=false：执行期错误（parse/run），error 为消息
```

**约束**（与 `/go` 一致）：

- 恰好 1 入参 1 返回值导出函数；`ValidateHTTPFuncs` 在**保存版本**时执行  — ✅
- 超时复用 `limits.query_timeout` 或独立 `gofunction_invoke_timeout`（默认 5s）  — ❌ 未做（沿用 RunJSON 解释器超时）
- 单测串行/小并发（semaphore 2），防解释器打爆 CPU  — ❌ 未做
- 审计：`kind=gofunction_test`，只记 name/version/function/status，不记 body  — ⚠️ `kind=gofunction` + detail=test；不记 body ✅

### 5.4 对外调用面 `/go`（生产）

```http
POST /go/:projectID/:name/:functionName
# body = JSON 入参；响应 = 返回值 JSON（保持无 envelope，便于 SDK）
# 始终执行 active_version；active_version=0 → 409 no_active_version
# 默认忽略 ?version=（即使配置打开也仅调试用途，文档标注「生产勿用」）
```

**定时任务 Runner（必须对齐）**

- 执行前：`ResolveActiveSource(project, name)` → `{version, source}`  
- Job 记录 `last_status=error` / `last_error=no_active_version`（若未发布）  
- 运行日志 `sys_go_func_invokes.channel=cron`，`version` 记**实际跑的 active**

### 5.5 版本生命周期状态机

```text
POST create ──► vN 存在
                    │
      activate ─────┤
                    ▼
              active_version = N     （仅一；CAS：UPDATE … SET active_version=N
                                      WHERE id=? AND active_version=旧值，冲突则重读）
                    │
      activate vM ──┘  → 旧 active 变普通历史版（无状态字段）
```

版本行**永不 UPDATE source**；「改代码」= `POST versions` 追加 `vN+1`。

**并发**

| 场景 | 策略 |
|---|---|
| 两人同时建版 | MAX+1 重试；最终 version 仍连续唯一 |
| 两人同时 activate | CAS 更新 `active_version`；后写覆盖前写（最后点击生效），响应返回最终值 |
| 测试 × 发布并行 | test 只读指定 version 行，与 activate 无锁耦合 |

### 5.6 中间件与权限

- 管理/test：沿用 `AuthMiddleware`（JWT/API Key）+ `Require(database:*)`  
- `/go`：沿用对外调用组（允许 API Key；登录态可选）  
- `writable=false` 实例：test / 建版 / activate 返回 503（只读实例仍可 **GET** 版本与列表）  
- admin 角色（login-auth-plan 只读）：`Require(database:write)` 天然挡住 test/activate

---

## 6. UI 详细设计

### 6.1 云函数列表（增加版本列 + 测试）

```text
┌────────────────────────────────────────────────────────────────────────────────┐
│ 云函数列表          共 3 个文件 · 调用前缀 POST /go/{pid}/{name}/{Fn}          │
│ [刷新] [新建云函数]                                                            │
├──────────┬──────────┬────────┬──────────┬──────────┬──────────────────────────┤
│ 文件     │ 导出函数 │ 生效版 │ 最新版本 │ 更新时间 │ 操作                     │
├──────────┼──────────┼────────┼──────────┼──────────┼──────────────────────────┤
│ hello.go │ Hello ★  │ v3 ●   │ v3       │ 09-24    │ 测试 查看 版本 编辑 删除│
│          │ Ping     │        │          │          │                          │
│ pay.go   │ Charge   │ v1 ●   │ v2       │ 09-23    │ 测试 查看 版本 编辑 删除│
└──────────┴──────────┴────────┴──────────┴──────────┴──────────────────────────┘
  「生效版」Badge：v3 ●（绿点=已发布）；最新>生效时提示「有未发布版本 v4」
  「测试」主按钮（默认可点）：打开调试台 Modal
  「版本」抽屉/弹层：版本时间线 + 设为生效 + 查看源码
```

列宽沿用 `sb-col-*`：文件 `sb-col-name` · 导出 `max-w-md` · 生效/最新 `sb-col-sm` · 时间 `sb-col-md` · 操作 `w-56`。

### 6.2 测试台 Modal（Postman 风格）— 核心

> 组件：`components/modal/GoFuncTestModal.vue`  
> 外壳：`SbModal` + `:max-width="960"` + `hideFooter`（操作在右栏内部）  
> 布局：左右分栏，最小高度约 480px；窄屏（≤768px）上下堆叠（先左后右）  
> **版本只在左栏切换**（需求：左侧导出函数名 + 版本），顶部仅展示只读徽标

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│  测试 · hello.go                                        发布状态 v3 ● [×] │
│  左栏选择「版本 / 导出函数」；试跑不会改变生效版本。                          │
├─────────────────────────┬────────────────────────────────────────────────────┤
│  版本                   │  请求                                            │
│  ┌────────────────────┐ │  POST [ /go/0000…02/hello/Hello    ] [复制]       │
│  │ ● v3  09-24 生效   │ │  对外 URL · 调用方需 Authorization: Bearer Key  │
│  │ ○ v2  09-22        │ │  ┌────────────────────────────────────────────┐  │
│  │ ○ v1  09-20        │ │  │ Body (JSON)                    { } 校验    │  │
│  └────────────────────┘ │  │ ┌────────────────────────────────────────┐ │  │
│                         │  │ │ {                                      │ │  │
│  导出函数（v3）          │  │ │   "name": "world"                      │ │  │
│  ┌────────────────────┐ │  │ │ }                                      │ │  │
│  │ ● Hello            │ │  │ └────────────────────────────────────────┘ │  │
│  │ ○ Ping             │ │  │                                            │  │
│  └────────────────────┘ │  │ [发送请求] ← 管理端 test API（当前登录态）   │  │
│                         │  │ 耗时 12ms · version=3 · channel=test       │  │
│  提示                   │  └────────────────────────────────────────────┘  │
│  POST+JSON，1 参 1 返回 │  响应                                            │
│                         │  200 OK · application/json                        │
│                         │  ┌────────────────────────────────────────────┐  │
│                         │  │ {                                          │  │
│                         │  │   "message": "hi world"                    │  │
│                         │  │ }                                          │  │
│                         │  └────────────────────────────────────────────┘  │
└─────────────────────────┴────────────────────────────────────────────────────┘
```

**左栏（版本 + 导出函数）— 唯一版本入口**

| 区块 | 内容 | 交互 |
|---|---|---|
| 版本 | `vN + 时间 + ● 生效`（可滚动，上限 50） | 点选切换；重新拉该版 exports；**试跑不改生效版** |
| 导出函数（vN） | 该**所选版本** `exports[]`；单选 | 选中后右栏 URL 尾段 = `{FunctionName}` |
| 生效标记 | 版本行 `● 生效` 唯一 | 标题右侧只读徽标「发布状态 v3」 |

**右栏（Postman 请求区）— URL 语义见 D11**

| 区块 | 内容 | 交互 |
|---|---|---|
| URL 行 | 只读**对外** `POST /go/{pid}/{name}/{Fn}` | 「复制」；caption 注明外部调用需 API Key |
| Body | JSON 文本域（`sb-mono`）；draft 仅 sessionStorage（key 含 pid/name/ver/fn） | 「校验」JSON.parse；无效红框 |
| 发送 | 调 `POST …/versions/{ver}/test`（**不是**上面展示的 URL） | loading；携带 `function_name` + `body` |
| 响应头 | `status_code` Badge（2xx 绿 / 4xx 橙 / 5xx 红）+ `duration_ms` + `version` | — |
| 响应体 | pretty JSON；`ok=false` 用 Alert 展示 `error` | 复制响应；服务端不存 body/响应原文 |

### 6.3 版本管理小弹窗（列表「版本」按钮）

```text
┌ 版本 · hello.go ───────────────────────────┐
│ v3  2026-09-24  [● 生效]  发布修复         │
│     [设为生效] [查看源码] [复制路径]       │
│ v2  2026-09-22  增加 Ping                  │
│     [设为生效] [查看源码]                  │
│ v1  2026-09-20  初始版本                   │
│     [设为生效] [查看源码]                  │
│              [关闭]                        │
└────────────────────────────────────────────┘
```

### 6.4 保存流程（编辑器 → 版本）

```text
新建云函数 ──► Monaco 写源码 ──► 保存 ──► 创建 sys_go_funcs + v1（默认 activate）
编辑源码   ──► 加载「生效版 source；无则最新版」──► 保存为新版本 vN+1
           ──► 勾选「保存后设为生效」（默认勾选）
「设为生效」──► POST versions/:ver/activate ──► /go 与定时任务立即切换
```

- 打开**历史版**源码：只读 Monaco + Banner「历史版本 v2 · 不可编辑；可测试 / 设为生效」。  
- `GoFunctionModal` footer：「取消 / 保存为新版本」；成功 toast：`已保存 v4`（生效则追加「并已设为生效」）。

---

## 7. API 契约汇总（snake_case）

### 7.1 `GoFunctionItem`（列表/详情）

```jsonc
{
  "id": "…",
  "name": "hello",
  "file": "hello.go",
  "description": "",
  "active_version": 3,
  "latest_version": 3,
  "exports": ["Hello", "Ping"],          // 生效版导出；无生效版则为最新版
  "versions": [                           // 详情才返回；列表可省略
    { "version": 3, "exports": ["Hello","Ping"], "note": "fix", "created_at": "…", "active": true }
  ],
  "created_at": "…",
  "updated_at": "…"
}
```

### 7.2 创建实体 / 新建版本

```jsonc
// POST /gofunctions
{ "name": "hello", "source": "package main\n…", "note": "初始", "activate": true }

// POST /gofunctions/:name/versions
{ "source": "…", "note": "增加 Ping", "activate": true }

// POST /gofunctions/:name/versions/:ver/activate
{}

// POST /gofunctions/:name/versions/:ver/test
{ "function_name": "Hello", "body": { "name": "world" } }
```

---

## 8. 时序（调试台试跑）

```text
用户                GoFuncTestModal           Echo                    systemdb/gofunction
 │ 点「测试」          │                        │                        │
 │──────────────────►│ open + GET versions ──►│ auth + Require(r)      │
 │                   │◄── v3 active, exports ──│◄── SELECT versions ────│
 │ 选 Hello / 改 JSON │                        │                        │
 │ 点「发送」         │                        │                        │
 │──────────────────►│ POST versions/3/test ─►│ Validate + RunJSON     │
 │                   │                        │  (source of v3)        │
 │                   │◄── 200 + data/duration │                        │
 │ 看状态码/JSON      │ 渲染响应面板           │ 记 sys_go_func_invokes │
```

---

## 9. 实施阶段（待批准后实施）

| Phase | 内容 | 交付 | 状态 |
|---|---|---|---|
| P1 | 迁移 v34–v36；`systemdb` gofuncs/versions/invoke 仓储 | 单测：版本追加、activate、唯一 version | ✅ 已完成 |
| P2 | 重写管理 API（实体+versions+activate+test） | curl 验收；`/go` 走 active_version | ✅ 已完成 |
| P3 | UI 列表版本列 + 版本弹窗 + 保存为新版本 | 列表/发布/回滚可用 | ✅ 已完成 |
| P4 | **测试台 Modal**（Postman 布局） | 左版本/导出、右 URL/Body/响应 | ✅ 已完成 |
| P5 | 指标/审计/文档 + mock 对齐 | `npm test` / `go test` 全绿 | ✅ 已完成（go test 全绿；UI 251 用例全绿，覆盖率 ≥95%） |

---

## 10. 验收标准

| # | 场景 | 期望 | 状态 |
|---|---|---|---|
| A1 | 创建云函数 | 生成 v1 且默认生效；列表「生效版 v1」 | ✅ 已完成 |
| A2 | 再次保存源码 | 生成 v2；可选「保存后生效」；不勾则生效仍为 v1 | ⚠️ 部分：生成 v2 ✅；保存后总是设为生效（API 支持 `activate`，UI 无勾选框） |
| A3 | 版本回滚 | 对 v1「设为生效」后 `/go` 立即执行 v1 源码 | ✅ 已完成 |
| A4 | 列表「测试」 | 打开 Postman 式 Modal，非全页 | ✅ 已完成 |
| A5 | 左栏导出 | 随所选版本变化；选中函数 URL 尾段正确 | ✅ 已完成 |
| A6 | 右栏请求 | URL 可复制；JSON 校验；发送后展示状态码 + 耗时 + pretty JSON | ⚠️ 部分：URL 复制 / JSON 校验 / 状态码 Badge（2xx 绿·4xx 橙·5xx 红）/ 耗时 / pretty JSON 均 ✅；缺「复制响应」按钮 |
| A7 | 测历史版 | 试跑 v2 成功且 `active_version` 不变 | ✅ 已完成 |
| A8 | 生产调用 | `/go` 与**定时任务**均跑 active；无 active → 409 / job error=`no_active_version` | ⚠️ 部分：`/go`→409 `no_active_version` ✅；cron runner → `no_active_version` 失败 ✅；缺 `sys_go_func_invokes.channel=cron` 流水 |
| A9 | 权限 | admin 只读无「测试/发布」；系统项目禁用写与测试 | ✅ 已完成 |
| A10 | 安全 | 日志/审计无源码与 body 原文；测试台不把 API Key 写进前端 | ✅ 已完成 |
| A11 | 版本上限 | 第 51 个版本被拒绝，提示先清理 | ✅ 已完成（`version_limit_exceeded`） |
| A12 | URL 语义 | 复制的是 `/go` 对外路径；页面「发送」打到 test 管理端 | ✅ 已完成 |

---

## 11. 风险与边界

| 风险 | 缓解 |
|---|---|
| 解释器试跑打满 CPU | test 单独 semaphore + 硬超时；与 `/go` 分配额 |
| 版本表膨胀 | 项目级版本上限（如每函数 50）；超出拒绝并提示删历史（软删可选） |
| 测试误当线上 | UI 明确「试跑不影响生效版」；`/go` 默认忽略 `?version` |
| 旧数据 | 按 D1 直接丢弃；文档写明升级需重建云函数 |

---

## 12. 已拍板的默认（原开放问题）

| 问题 | 默认 | 状态 |
|---|---|---|
| 是否持久化示例 Body | **否**（仅 sessionStorage，按 pid/name/ver/fn） | ✅ 已按默认实现 |
| `/go?version=n` | **默认关**；实现保留配置开关仅供调试，文档标注生产勿用 | ⚠️ 默认忽略 `?version` ✅；未保留配置开关 |
| 版本 diff UI | **本期不做**；版本弹窗提供「查看源码」只读即可 | ⚠️ diff 不做 ✅；版本弹窗为「复制源码」而非只读查看 |
| Request Headers 编辑 | **本期不做**（仅固定 JSON body；鉴权走控制台登录态 / 外部 API Key） | ✅ 已按默认不做 |

---

## 13. 复查结论（2026-09-24）

对初版 plan 复查后已吸收的优化：

1. **表名对齐**：D2 与 §4 统一为 `sys_go_funcs` / `sys_go_func_versions`（不再混用旧名）。  
2. **生产调用语义**（§2.1 / D8）：`/go` **与定时任务**都现查 `active_version`，避免回滚后 cron 仍跑旧版。  
3. **D11 URL 语义**：右栏展示对外 URL ≠ 测试请求通道，防止误导与前端塞 Key。  
4. **测试台布局**：版本**只在左栏**（更贴需求原文），去掉顶部重复选择器。  
5. **并发与上限**：version=MAX+1 重试；activate 用 CAS；每函数 50 版硬上限（D3 / §5.5）。  
6. **派生字段**：`latest_version` / `exports` / 编辑器 source 取值规则写清（§4.2 / D12）。  
7. **验收补强**：A8–A12 覆盖 cron、URL 语义、Key 安全、版本上限。

---

## 14. 执行状态复盘（2026-09-24，代码库对照核实）

> 结论：**主体全部落地**（P1–P5 完成，`go test ./...` 全绿，UI `npm test` 251 用例全绿，覆盖率 Statements 98.8% / Branches 95.2% / Functions 96.3%）。
> 存在少量体验层简化与 3 处未做项，均不影响主路径；明细如下。

### 14.1 决策 D1–D12

| # | 决策 | 状态 | 证据 |
|---|---|---|---|
| D1 | 重设计，不兼容 | ✅ | 迁移 v34–v36 新表；旧 `sys_gofunctions` 不再读写 |
| D2 | 实体 + 版本快照分离 | ✅ | `sys_go_funcs` / `sys_go_func_versions` |
| D3 | 版本号单调递增 / 不可变 / 上限 50 | ✅ | `AppendGoFuncVersion` MAX+1 重试 ≤3；`ErrVersionLimit` |
| D4 | 唯一生效版 | ✅ | `active_version`；`ResolveActiveSource` |
| D5 | 测试可打任意版本 | ✅ | `POST …/versions/:ver/test` 指定版本 |
| D6 | Postman 布局 Modal | ✅ | `GoFuncTestModal.vue`（SbModal 960 + hideFooter） |
| D7 | 管理面测试走专用端点 | ✅ | 登录态 + `database:write` |
| D8 | 生产调用只认生效版 | ✅ | `/go` 与 cron 均 `ResolveActiveSource`；`?version` 被忽略 |
| D9 | exports 按版本快照 | ✅ | `exports_json` 落版本行；左栏随所选版本变化 |
| D10 | 列表列增加版本信息 | ✅ | 生效版 / 最新版本 / 导出 / 更新时间 / 操作 |
| D11 | 展示 URL ≠ 测试通道 | ✅ | 只读 `/go/...`；发送打 test 管理端 |
| D12 | 无生效版 exports 取最新 + 标注 | ✅ | DTO `published` 字段；UI「未发布」Badge |

### 14.2 后端（§4 / §5）

| 项 | 状态 | 说明 |
|---|---|---|
| 迁移 v34 `sys_go_funcs` | ✅ | `systemdb/migrate.go` |
| 迁移 v35 `sys_go_func_versions` | ✅ | 同上 |
| 迁移 v36 `sys_go_func_invokes` | ✅ | 同上；不存 body/响应原文 |
| 仓储 CRUD / Append / Activate / Resolve / Record | ✅ | `systemdb/gofuncs.go` |
| 管理 API 10 端点 | ✅ | `router.go` 全量挂载 |
| 错误码 `gofunction_not_found` / `invalid_gofunction_name` | ✅ | — |
| 错误码 `version_not_found` / `invalid_exports` / `version_exists` | ⚠️ | 实现用 `not_found` / `gofunction_signature_invalid` 统一映射；未单独出码（语义等价，文案更简） |
| 额外错误码 `version_limit_exceeded` | ✅ | 对应 A11 |
| test 响应 envelope（ok/status/duration/version/active/data/error） | ✅ | 200 包裹，业务错在 payload |
| 保存时 `ValidateHTTPFuncs` | ✅ | `validateSource` |
| test 独立超时 `gofunction_invoke_timeout`（默认 5s） | ❌ 未做 | 复用 `RunJSON` 内置解释器超时；无独立配置 |
| test 并发 semaphore(2) | ❌ 未做 | 未加限流（单机控制台场景风险可接受） |
| 审计 `kind=gofunction_test` | ⚠️ | `kind=gofunction` + detail `test xxx.go vN`；不记 body ✅ |
| `/go` 409 `no_active_version` | ✅ | — |
| cron 目标校验 + `no_active_version` | ✅ | `cronjob_handler.go` 校验；`cron_runner.go` 运行时失败 |
| cron 流水 `channel=cron` | ❌ 未做 | `RecordGoFuncInvoke` 仅 `go`/`test` 两种 channel |
| `?version=` 配置开关 | ❌ 未做 | 始终忽略 query（比「默认关」更严格） |
| activate CAS | ⚠️ | 直接 UPDATE（DuckLake 无真 CAS；后写覆盖，语义同 plan） |
| writable=false → 503；系统项目禁写/测 | ✅ | — |

### 14.3 UI（§6）

| 项 | 状态 | 说明 |
|---|---|---|
| 列表版本列 + 测试/版本按钮 | ✅ | `GoFunctions.vue` |
| 「有未发布版本」提示 | ✅ | latest > active 时展示 |
| 测试台左栏（版本 + 导出函数） | ✅ | 版本只在左栏切换 |
| 测试台右栏 URL / 复制 / caption | ✅ | D11 语义 |
| Body sessionStorage（pid/name/ver/fn） | ✅ | — |
| JSON 校验按钮 + 无效红框 | ✅ | — |
| 发送 → test API + loading | ✅ | — |
| 响应头：status Badge 三色 + duration + version | ✅ | 耗时行含 `channel=test` |
| 响应体 pretty JSON + `ok=false` Alert | ✅ | — |
| 复制响应 | ❌ 未做 | 仅 URL 复制 |
| 版本弹窗时间线 + 设为生效 | ✅ | `GoFuncVersionsModal.vue` |
| 版本弹窗「查看源码」只读 | ⚠️ | 实为「复制源码」到剪贴板（无只读 Monaco 查看） |
| 保存为新版本（footer 文案 + toast） | ✅ | `GoFunctionModal.vue` |
| 「保存后设为生效」勾选（默认勾选） | ❌ 未做 | 固定 `activate: true` |
| 历史版只读 Monaco + Banner | ❌ 未做 | view 模式只读有；无按版本打开入口 |
| 版本 `note` 备注编辑 | ⚠️ | 后端字段与展示有；保存 UI 未填 note |

### 14.4 未做项汇总（可作 v3.1 候选）

1. test 独立超时 `gofunction_invoke_timeout` + 并发 semaphore(2)（§5.3 约束）。
2. cron 执行写 `sys_go_func_invokes`（`channel=cron`，§2.1 / §5.4）。
3. 测试台「复制响应」按钮（§6.2 响应体行）。
4. 「保存后设为生效」勾选 + 历史版只读 Banner（§6.4）。
5. 版本弹窗「查看源码」只读视图（现为复制）。
6. `/go?version=` 配置开关（§12 默认关，现为硬忽略）。
7. 细分错误码 `version_not_found` / `invalid_exports` / `version_exists`（现统一映射）。