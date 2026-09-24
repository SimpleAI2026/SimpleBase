# 数据库默认就绪：去掉打开 / 关闭

> **状态**：plan-only（本文件只规划，不改产品代码）。  
> **日期**：2026-09-24  
> **Verified against**：`main` @ `1a1bf75`（`Merge branch 'cursor/docs-wiki-polish-3266'`）。行号对应该提交。  
> **决策**：数据库列表不再提供「打开 / 关闭」。创建成功即 `ready`、可直接 SQL。连接句柄的空闲关闭仍是进程内部行为。  
> **范围**：控制台操作、公开 HTTP、catalog 对外状态、JS SDK、mock、测试，以及仍把 open/close 写成用户步骤的文档。  
> **本计划不改**：DuckLake / S3 对象格式、空闲淘汰、`max_open`、设置页。

---

## 0. 一句话目标 / One-sentence goal

用户心智是 **创建 → 可用**。列表操作里没有「打开 / 关闭」。进程仍可在空闲时关掉本地句柄；下一次 SQL / 集合请求自己 `Acquire`，不需要用户再点一次。

---

## 1. 背景与核实 / Baseline

对照本仓库当前代码，而不是旧计划里的九态状态机原文。

### 1.1 假设里成立的部分

| 点 | 位置 |
|---|---|
| 列表操作有「打开」「关闭」 | `ui/src/pages/Databases.vue` 约 103–121 行。仅非 admin 项目。关闭 toast：`${name} 已关闭（数据保留）` |
| 按钮调用 `api.databases.open` / `close` | 同文件 `openDb` / `closeDb`（约 412–429 行） |
| HTTP | `POST /v1/projects/:projectID/databases/:databaseID/open` 与 `.../close`，挂在 `internal/api/router.go` 204–205 行 |
| open | `DatabaseHandler.OpenDatabase`：`Acquire(ReadWrite)` 后立刻 `Release`，再 `SetDatabaseReady`（`internal/api/database_handler.go` 188–221 行） |
| close | `Registry.CloseDatabase`：没有活跃引用时释放本地句柄，不删 S3（`internal/database/registry/registry.go` 190–211 行）。HTTP **204 无 body** |
| 九态常量还在 | `internal/catalog/model.go` 20–29 行：`creating` / `opening` / `ready` / `closing` / `closed` / `degraded` / `deleting` / `deleted` / `recovering` |
| 空闲关闭与缓存淘汰仍关句柄 | `Registry.CloseIdle`、`Registry.Shutdown`、`cache.Manager.Evict` → `Closer.CloseDatabase` |

### 1.2 核实后要改写的部分

这些和「今天用户必须先打开才能用」不一致，实现时以这里为准。

1. **真实创建路径已经停在 `ready`。** `catalog.Service.CreateDatabase` 在 catalog（及可选 descriptor）写成功后调用 `SetDatabaseReady`，返回值 `Status = ready`（`internal/catalog/service.go` 141–149 行）。`plan/planv2.0/proto-http.md` §3.1 也写了：HTTP 201 一般已是 `ready`，`creating` 只在请求内部短暂存在。open 把卡住的 `creating` 推到 `ready`，是历史补救，不是正常创建步骤。
2. **真实 close 不写 catalog 状态。** `CloseDatabase` handler 只校验归属并调用 `Registry.CloseDatabase`，不 `TransitionDatabase`。关完之后列表状态仍是 `ready`。toast「数据保留」只是文案；服务端本来就不会删数据。
3. **`closed` 主要是 mock 写出来的。** `ui/src/services/mock.js` 的 `close` 把 `db.status = 'closed'`，`open` 再改回 `ready`。生产 `service.go` 里没有转到 `opening` / `closing` / `closed` 的写入。这三个值出现在删除 / 降级的 **from** 列表，以及单测直接调 `TransitionDatabase`。
4. **今天的 open 也救不了 `closed`。** `SetDatabaseReady` 的 from 只有 `creating` / `opening` / `recovering`（`service.go` 382–390 行）。`closed` 会落到 `ErrInvalidState` 并被吞掉，响应仍是 `closed`。控制台 `isReady` 只认 `status === 'ready'`，于是 SQL、展开集合、新建集合保持禁用。
5. **`Registry.Acquire` 并不拒绝 `closed`。** `validateAccess` 只拒绝 `deleting` / `deleted` 和 `degraded`（`registry.go` 174–187 行）。句柄被 `CloseIdle` 删掉之后，下一次 `Acquire` 会重新 `factory.Open`。挡住「已关闭库不能查询」的是控制台，不是引擎。
6. **云 Agent 不调用 open/close。** `internal/cloudagent` 的工具是 `list_databases`、`list_collections`、`readonly_sql`。这条假设丢弃，实现时不要改 Agent 工具面。
7. **仓库里没有 OpenAPI 文件。** 契约在 `plan/planv2.0/proto-http.md` 与 `plan/planv2.0/proto.http`。`proto-http.md` 里 §3.1 的 open/close 表重复出现四次（约 123、609、1116、1602 行），改文档时要全部改到。
8. **没有独立 i18n 目录。** 「打开」「关闭」「已关闭」「打开中」「关闭中」写在 `Databases.vue` 和 `ui/src/lib/status.ts`。
9. **descriptor 状态不是产品状态。** 创建时 S3 descriptor 写成 `objectstore.StatusCreating`（`service.go` 125 行），随后没有把 descriptor 改成 `ready`。UI 读的是 catalog `sys_databases.status`。本计划不改对象内容。

### 1.3 现在用户实际看到什么

非 admin 数据库列表的操作是：查看数据、SQL、打开、关闭、新建集合、删除。

| 操作 | 何时可点 | 去掉打开/关闭之后 |
|---|---|---|
| 查看数据 / 收起 | `status === 'ready'` | 保留。未就绪 tooltip 仍是「数据库未就绪」 |
| SQL | 同上；admin 为只读控制台 | 保留 |
| 打开 | `isOpenable`：`ready` / `closed` / `degraded` | **删除** |
| 关闭 | 仅 `ready` | **删除** |
| 新建集合 | `ready` | 保留 |
| 删除 | `status !== 'deleting'`；确认文案仍是「删除为异步操作」 | 保留。与关闭无关 |
| admin「受保护」 | 系统库隐藏新建、打开、关闭、删除、新建集合 | 保留。本来就没有打开/关闭 |

空列表没有单独的「库已关闭」空态。Dashboard 用同一套 `statusText` 画状态徽章（`ui/src/pages/Dashboard.vue`）。

mock 与真实 API 不一致：`mock.js` 的 `create` 返回 `creating`，必须再点打开才变成 `ready`。真实 API 创建成功已经是 `ready`。实现时 mock 要跟真实行为对齐，否则本地 `VITE_USE_MOCK=true` 仍会看起来「创建完不能用」。

---

## 2. 产品决策 / Decisions

| # | 决策 | 含义 |
|---|---|---|
| 1 | 没有用户级打开/关闭 | 列表、SDK、HTTP 都不再提供这个动作 |
| 2 | 创建成功即可用 | 心智是 `ready`。不要求用户预热 |
| 3 | 「始终可用」≠「句柄永不关闭」 | `CloseIdle` / 缓存淘汰 / `Shutdown` 保留。下次查询透明重新打开 |
| 4 | 关闭 ≠ 删除 | 删除仍是软删：`deleting` → 关句柄 → 清平面 B 前缀 → `deleted` + `deleted_at`。close 从未删 S3，去掉之后也不许用别的按钮偷偷做这件事 |
| 5 | 公开路由一次删除 | 见 §4。不留一个「只为了按钮」的管理 API |
| 6 | `Registry.CloseDatabase` 保留 | 删除路径和缓存淘汰还在调用。去掉的是 HTTP/UI/SDK 上的手动入口 |

---

## 3. 产品 / UX

默认心智，写进列表副标题或空态时用这一句即可，不要再解释连接池：

> 新建数据库后即可查询、建集合。不需要打开或关闭。

### 3.1 从列表拿掉的东西

- `Databases.vue` 的「打开」「关闭」按钮、tooltip、`openDb` / `closeDb`、`isOpenable`。
- toast：`已打开`、`已关闭（数据保留）`、`打开失败`、`关闭失败`。
- 仅因关闭才存在的展示：`statusText` 的 `已关闭`，以及 `opening` →「打开中」、`closing` →「关闭中」。没有单独的 closed 空态要删；不要新做一块「如何打开数据库」的说明。
- `RocketIcon` / `PowerIcon` 若不再被该页使用，去掉 import。

### 3.2 仍然可见

- 刷新、新建数据库（非 admin）。
- 状态徽章：至少「就绪 / 创建中 / 降级 / 删除中」。`已删除` 行因 `deleted_at IS NULL` 过滤，列表里通常看不到（`sql_repository.go` 174–175 行）。
- SQL、查看数据、新建集合：仍只在 `ready` 时启用。`creating`（创建请求尚未返回时的极短窗口）、`degraded`、`deleting` 继续显示「数据库未就绪」，这不是打开按钮的替代品。
- 删除与「受保护」。删除确认可以继续说清理是异步/提交后刷新；不要把「数据保留」挪到删除文案上，那是旧关闭 toast 的句子。

### 3.3 创建反馈

成功 toast 今天是 `数据库 ${name} 创建成功（状态：${statusText(db.status)}）`。创建响应已是 `ready` 时，文案应让人看出可以直接使用，例如带上「就绪」，而不是暗示还要再操作一步。mock 的 `create` 必须直接返回 `ready`，与 `catalog.Service.CreateDatabase` 一致。

---

## 4. API 表面

### 4.1 假设与建议

**假设：** 本仓库之外没有已发布、需要单独弃用窗口的 open/close 客户端。依据：

- 没有 OpenAPI 产物。
- 进程内调用方是控制台 `http-api.ts`、`mock.js`、`packages/js-sdk`（`src/databases.ts` 与已提交的 `dist/index.js`）。
- 云 Agent 不调用这两条路由。
- 真实 close 不落库状态，外部调用方并没有在服务端留下一个「已关闭」生命周期可依赖。

**建议：一次删除，不要先 deprecate 再删。** 路由从 `mountV1Routes` 卸掉即可。

卸掉之后的行为：SPA fallback 只注册了 `GET /*`（`internal/web/web.go` 60 行）。未注册的 **POST** 走 Echo 404，经 `errorHandler` 变成 JSON `404`，`error.code = not_found`（`internal/api/error.go` 的 `httpStatusToCode`）。不是 HTML，也不是 `410`。验收按这个写。

不推荐多留一个发行版的 `410 Gone`：没有外部契约，多一个临时 handler 只会把「这个动作还存在」写回 API。若产品负责人后来确认有仓库外客户端，再单独加一版明确的 `410`；那不是本计划的默认路径。

### 4.2 要删的公开入口

| 入口 | 今天 | 之后 |
|---|---|---|
| `POST :p/databases/:databaseID/open` | `DatabaseAdmin`，200，`DatabaseResponse` | 不挂载 → POST `404 not_found` |
| `POST :p/databases/:databaseID/close` | `DatabaseAdmin`，204 | 同上 |
| `packages/js-sdk` `databases.open` / `close` | `src/databases.ts`、`dist/index.js`、`dist/index.d.ts` | 从 `DatabasesApi` 删除，并重新生成 dist |
| 控制台 `http-api.ts` / `types.ts` 的 `open` / `close` | 与 mock 同一 `Api` 接口 | 两边一起删，避免签名分叉 |

保留：`POST/GET databases`、`GET/DELETE :id`、`query` / `execute` / `batch`、集合与文档 API。

### 4.3 权限

open/close 与创建、删除相同，都是 `auth.DatabaseAdmin`（`database:admin`）。没有单独的 open scope。删路由 **不** 删除 `DatabaseAdmin`，创建和删除还要用。

`instance.writable = false` 时，这两条和 create/delete 一样返回 `503 writer_unavailable`。路由删除后，这个 503 不再属于 open/close；create/delete/execute 的只读实例行为不变。`plan/planv2.0/proto-http.md` 里「写类接口（create/open/close/delete）」改成「create/delete」。

### 4.4 handler 接口

`DatabaseHandler` 的 `DatabaseService` 上，仅为 open/close 存在的方法可以从 **handler 接口** 拿掉：

- `CloseDatabase`（handler 侧）
- `SetDatabaseReady`（只有 `OpenDatabase` 通过 handler 调用；创建路径在 `catalog.Service` 内部）
- `Acquire`（handler 侧只有 open 用；SQL/Data 用的是另一套 `SQLService.Acquire`）

`dbServiceAdapter` 里对应的三个方法若再无引用，一并删除。`registry.Registry.CloseDatabase` **不** 在此列。

---

## 5. Catalog / 状态模型

`sys_databases.status` 是 `VARCHAR`，没有 CHECK（`internal/systemdb/migrate.go` version 3）。改枚举不需要改表结构，需要一次数据修正。

### 5.1 各状态怎么处理

| 状态 | 决定 | 理由 |
|---|---|---|
| `ready` | **保留**，且是创建成功后的正常值 | 已实现。列表、SQL、集合都以它为「可用」 |
| `creating` | **保留**，仅作创建请求内部的短暂状态 | 插入后、`SetDatabaseReady` 之前。成功响应不应停在这里。进程在这两步之间崩溃时，今天靠 open 补救；open 删除后要有内部补救（§5.2） |
| `degraded` | **保留，且不要自动改成 ready** | descriptor 失败等会走到这里（`degradeAfterCreateFailure`）。`Acquire` 拒绝并返回 `ErrDatabaseNotReady`。这是故障，不是「关着」 |
| `deleting` | **保留** | 软删进行中。UI 禁用再次删除 |
| `deleted` | **保留** | `MarkDatabaseDeleted` 写入，并设 `deleted_at`。列表因 `deleted_at IS NULL` 隐藏。与 close 无关 |
| `closed` | **对用户消失** | 生产代码不写入。mock 写入。已有行改成 `ready` |
| `opening` | **对用户消失** | 无生产写入。已有行改成 `ready` |
| `closing` | **对用户消失** | 无生产写入。已有行改成 `ready` |
| `recovering` | **对用户消失** | 无生产写入。`backups` / `restore` 路由已按 proto-http 勘误删除。已有行改成 `ready`。不在本计划恢复备份功能 |

软删与关闭必须在实现说明和测试里分开写：

- 关闭（即将删除的 API）：不改 `deleted_at`，不调用 `DeletePrefix`，不把状态改为 `deleting` / `deleted`。
- 删除：`BeginDeleteDatabase` 的 from 今天包含 `closed`。迁移之后 from 改为仍能删的活状态：`creating` / `ready` / `degraded`，以及迁移完成前可能残留的 `opening` / `recovering`。`deleted` 保持终态，不能删回去变成可用库。

### 5.2 `closed` 与卡住的 `creating` 如何变成可用

推荐 **启动时一次幂等 UPDATE + 读取时兜底**，两步都做：

1. **一次迁移**（catalog 启动或新的 `systemdb` migration version，幂等）：

   ```sql
   UPDATE sys_databases
   SET status = 'ready', updated_at = CURRENT_TIMESTAMP
   WHERE deleted_at IS NULL
     AND status IN ('closed', 'opening', 'closing', 'recovering');
   ```

   不更新 `degraded` / `deleting` / `deleted`。不碰 S3。

2. **读取兜底**：`GetDatabase` / `ListDatabases` 若仍见到 `closed` / `opening` / `closing` / `recovering`，对响应按 `ready` 返回，并最好写回（覆盖滚动升级时旧进程刚留下的行）。这样不依赖用户再调一次已经不存在的 open。

3. **卡住的 `creating`：** 不要在每次 list 上把「正在创建」无条件改成 `ready`（descriptor 还没写完时会把失败窗口标成就绪）。替换 open 补救的规则：

   - 进程启动时，把 **本进程没有进行中的创建**、且仍为 `creating` 的行：若 descriptor 存储未启用（DevMode 无 descriptor）或 descriptor 已存在，则 `SetDatabaseReady`；若启用了 descriptor 但对象不存在，则 `SetDatabaseDegraded`，而不是假装就绪。
   - 正常 `CreateDatabase` 成功路径保持现状：函数返回前已经是 `ready`。

`SetDatabaseReady` 今天的 from 不含 `closed`。迁移 SQL 直接写 `ready`，不必把 `closed` 塞进 `SetDatabaseReady`，除非读取兜底想复用那个函数。若复用，from 要加上 `closed` / `opening` / `closing` / `recovering`，并且 **仍然不要** 从 `degraded` 转出。

`objectstore` 的 `StatusOpening` / `StatusClosing` / `StatusClosed` 常量可以在确认无引用后删除。不要为了对齐去回写已有 descriptor JSON。

---

## 6. Runtime（Registry / cache）

用户不控制连接池。下面这些保持内部 API：

| 能力 | 保留原因 |
|---|---|
| `Registry.Acquire` / `Lease.Release` | SQL、集合、数据 API、云 Agent 只读查询都靠它。entry 不存在时 `factory.Open` |
| `Registry.CloseIdle` | `active == 0` 且超过 `idleTimeout`（默认 5 分钟）关闭 ready handle |
| `Registry.Shutdown` | 进程退出 |
| `Registry.CloseDatabase` | `DeleteDatabaseSync`（`internal/api/adapter.go` 把 `a.registry.CloseDatabase` 传进 closer）、`cache` 淘汰（`internal/database/cache/evict.go`） |
| `cache.Manager.Evict` | 只淘汰非活跃库；先关句柄再删缓存目录 |
| `max_open` / `IdleTimeout` | 配置限额。本计划不改默认值 |

要从公开表面去掉的，只是「管理员手动 `CloseDatabase`」：

- 删除 `POST .../close` 与 handler。
- 不要新增替代的 admin/UI 路由。
- registry 方法、删除路径、淘汰路径留着。单测 `TestCloseDatabase_*` 继续测句柄，不测 HTTP。

**必须写明并做成测试的行为：** `CloseIdle`（或淘汰）之后 catalog 状态仍是 `ready`；下一次 `query` / `execute` / 集合读通过 `Acquire` 重新打开并成功。失败时错误是引擎/存储错误，不是「请先打开数据库」。

`degraded` 的 `Acquire` 拒绝保持不变。本计划不把降级库自动打开。

---

## 7. 前端清理

无 vue-i18n 文件。改组件内中文和 `status.ts`。

| 文件 | 动作 |
|---|---|
| `ui/src/pages/Databases.vue` | 删除打开/关闭按钮、`isOpenable`、`openDb`、`closeDb` 及相关 icon |
| `ui/src/lib/status.ts` | 删除 `opening` / `closing` / `closed` 的文案与 badge 分支。`recovering` 若不再出现，一并删除。保留 `ready` / `creating` / `degraded` / `deleting` / `deleted` |
| `ui/src/lib/status.test.ts` | 不再断言 `opening` / `closing` 为 outline；`statusText('closed')` 不应再是「已关闭」 |
| `ui/src/services/types.ts` | `DatabaseItem.status` 去掉 `opening` / `closing` / `closed` / `recovering`。注释不要再写「新建时为 creating（不是 active）」——成功创建对 UI 而言是 `ready`。`Api.databases` 去掉 `open` / `close` |
| `ui/src/services/http-api.ts` | 删除 `/open`、`/close` |
| `ui/src/services/http-api.test.ts` | 删除约 127–129 行的 open/close 调用 |
| `ui/src/services/mock.js` | `create` 返回 `ready`；删除 `open` / `close`。不要再把状态写成 `closed` |
| `ui/src/services/mock.test.ts` | 改为断言创建即 `ready`；删除 missing open/close |
| `ui/src/test/api-mock.ts` | 去掉 `databases.open` / `close` 的 mock |
| `ui/src/test/helpers.ts` | `closedDb` 改为 `degraded` 或 `creating` 夹具，供「未就绪」用例使用 |
| `ui/src/pages/Databases.test.ts` | 不再期望按钮「打开」「关闭」、`已关闭`、open/close 失败 toast |
| `ui/src/pages/pages-coverage.test.ts` | `isOpenable({ status: 'closed' })` 删除 |
| `ui/src/pages/interactions.test.ts` | 用例名里的 open/close 点击改为 SQL / 集合 / 删除 |
| `ui/src/coverage-gaps.test.ts` | 去掉 open/close reject 分支 |
| `ui/src/pages/Dashboard.test.ts` | 若用 `closedDb` 只为渲染徽章，改成仍存在的状态 |

`api.ts` 的 mock/http 切换不用新分支；两边实现同一个更小的 `Api`。

---

## 8. 后端清理

| 文件 | 动作 |
|---|---|
| `internal/api/router.go` | 删除 open/close 两行 |
| `internal/api/database_handler.go` | 删除 `OpenDatabase`、`CloseDatabase`；收窄 `DatabaseService` |
| `internal/api/adapter.go` | 删除仅服务 handler 的 `CloseDatabase` / `SetDatabaseReady` / `Acquire` 包装。`DeleteDatabaseSync` 仍传 `registry.CloseDatabase` |
| `internal/api/database_handler_test.go` | 删除 `TestOpenDatabase_*`、`TestCloseDatabase_Success`、`TestPlan_OpenDatabasePromotesCreatingToReady`。改成：创建 HTTP 201 为 `ready`；POST open/close 为 `404` 且 `error.code=not_found` |
| `internal/api/handler_branches_test.go` | 从 writable / not-found / 只读矩阵里去掉 open/close |
| `internal/api/adapters_test.go` | 不再把 handler `CloseDatabase` 当公开行为测 |
| `internal/catalog/service.go` | §5.2 的迁移与读取兜底；收紧 `BeginDeleteDatabase` 的 from。`SetDatabaseReady` 继续服务创建成功路径 |
| `internal/catalog/model.go` | 常量可留到迁移落地再删，避免半截编译。注释里的「见 plan.md 5.2 状态机」改为指向本文件，不要再描述 `ready → closing → closed` |
| `internal/catalog/*_test.go` | 直接 `TransitionDatabase(..., DatabaseClosed)` 的用例改为迁移：`closed` 行读出来是 `ready`，且能 `Acquire` + 查询。保留 deleting/deleted/degraded 用例 |
| `internal/objectstore/descriptor.go` | 仅当 `StatusOpening` / `StatusClosing` / `StatusClosed` 无引用时删除常量。不改已写入的对象 |
| `internal/database/registry/registry.go` | **不** 删除 `CloseDatabase` / `CloseIdle` / `Shutdown`。补或保持：idle close 后再次 `Acquire` 成功，且不修改 catalog status |
| `internal/database/cache/evict.go` | 注释第 4 行「只淘汰已关闭」容易读成产品状态 `closed`。改成「只淘汰 registry 中无活跃引用的库」。逻辑不变 |
| `internal/README.md` | `database_handler.go` 与 catalog 状态机那两句在文档阶段改掉（§9） |
| `packages/js-sdk/src/databases.ts` 与 `dist/*` | 删除 `open` / `close`。dist 已入库，必须一起更新 |
| 云 Agent | **不改**。已确认无 open/close 调用 |

`Operation.Kind` 注释里的 `"open" | "close"`（`model.go` 98 行）没有对应的生产 `AppendOperation`。不必回填历史审计行；新代码不要再写这两种 kind。

---

## 9. 文档（实现 PR 的清单）

本计划 PR **不** 改下列文件。实现功能时按清单改。历史计划（planv1 / 已落地的 planv2）加一行「open/close 产品面已废弃，见 `plan/planv3.0/database-always-open-plan.md`」，不要把旧设计文档改写成另一种架构。

用户能读到、必须改掉「先打开再使用」的：

- [ ] `docs/database/index.md`：去掉「创建 / 打开 / 关闭数据库」
- [ ] `docs/sdk/database-sql.md`：去掉 `sb.databases.open` / `close`
- [ ] `README.md` API 表：删除 open/close 两行
- [ ] `internal/README.md`：handler 列表与 `creating→opening→ready→closing→closed` 那句
- [ ] `plan/planv2.0/proto-http.md`：四处 §3.1 表、`DatabaseResponse` 示例里的九态、以及「create/open/close/delete」
- [ ] `plan/planv2.0/proto.http`：约 65–70 行「打开数据库 / 关闭数据库」
- [ ] `plan/planv2.0/js-sdk-plan.md`：约 117 行 `sb.databases.open / close`

只加废弃说明、不整篇重写的：

- [ ] `plan/planv2.0/databases-and-s3-plan.md`（生命周期仍写 create → open/close；`TestPlan_OpenDatabasePromotesCreatingToReady` 改为启动修复或读取兜底的测试名）
- [ ] `plan/planv2.0/db-ducklake-plan.md` 约 494 行（open = 预热 ATTACH）
- [ ] `plan/planv2.0/ui-plan-v2.md` 约 68、133 行（九态、open/close）
- [ ] `plan/planv1.0/plan.md`、`plan3.md`、`plan5.md`、`plan7.md` 里把 open/close 写成用户步骤的段落

`docs/ops/deployment.md` 里的 `max_open`、冷启动、进程 `Shutdown` 关闭连接是运维指标，**保留**。不要改成「用户可以关闭数据库」。`docs/database/ducklake.md` 是引擎长文里的 connection close，不是控制台按钮，不必为这个产品决策重写。

`docs/ops/migration.md` 的「仅能打开」指迁移校验，不是这个 API，不要误改。

---

## 10. 分阶段、风险、回滚

### 10.1 为什么不先只改 UI，也不先只删路由

| 顺序 | 问题 |
|---|---|
| 只先改 UI | mock 与历史 `closed` 行仍不是 `ready`，SQL 继续禁用，只是按钮没了，用户更无法操作 |
| 只先删 API | 旧控制台按钮会 toast「打开失败 / 关闭失败」 |
| 先 deprecate 一个版本 | 仓库内没有需要窗口的外部客户端；close 也不持久化状态 |

**实现用一个 PR，内部按下面的顺序提交或至少按这个顺序改：**

1. **Phase A — 状态兼容（先让旧行可用）**  
   §5.2 迁移 + 读取兜底 + 启动时处理残留 `creating`。此时路由可以还在，但不再被当成产品步骤。测试：`status=closed` 的行列表为 `ready`，且 query 成功。
2. **Phase B — 同时拿掉公开表面**  
   路由、handler、SDK、`http-api`、mock、`Databases.vue`、相关测试一起删。避免 UI 打到已删除路由。
3. **Phase C — 文档**  
   §9 清单。可以跟 B 同一 PR，也可以紧接着的文档 PR。不要把文档留在「请先打开数据库」。

Phase A 和 B 不要拆成两个已发布版本。拆开就会出现上表里的窗口。

### 10.2 风险

| 风险 | 处理 |
|---|---|
| 仓库外仍有人 POST open/close | 得到 JSON `404 not_found`。默认接受。若事后确认有客户端，再补 `410`，不在本计划第一刀做 |
| 把残留 `creating` 一律标 `ready`，但 descriptor 没写上 | 启动修复按 §5.2：无对象则 `degraded`，不要标就绪 |
| 把 `degraded` 迁成 `ready` | 禁止。降级库 `Acquire` 会失败，标成就绪会让 SQL 按钮可点然后报错 |
| 删除路径误用「关闭」语义 | 测试锁定：delete 设置 `deleted_at` 并清理平面 B；不存在的 close 不会 |
| 覆盖率只删测试、不补行为 | 用「创建即 ready」「旧 closed 可读可查」「idle 后查询成功」「POST open/close 为 404」替换被删用例 |
| `proto-http.md` 只改了第一处 | 文件内同一张表有四份 |
| JS SDK 只改 `src` 不改 `dist` | `dist` 已提交，调用方会继续打到 `/open` |

### 10.3 回滚

- 代码回滚：revert 实现 PR。路由回来，按钮回来。
- 数据：`closed|opening|closing|recovering → ready` 的 UPDATE **不需要 down migration**。这些状态没有对应的存储差异，revert 代码后行留在 `ready` 仍然正确，也符合本决策。
- 不涉及 S3 对象，没有存储格式回滚。

---

## 11. 验收 / DoD

### 11.1 产品

- [ ] 数据库列表没有「打开」「关闭」，tooltip / toast / 空态里也没有。
- [ ] 新建成功后列表为「就绪」，不点任何打开动作即可打开 SQL 并跑通一条查询；新建集合可用。
- [ ] admin 系统库仍是只读 SQL + 查看数据 +「受保护」，不能删除。
- [ ] 删除仍二次确认，成功后行进入删除流程；数据面清理与「曾经的关闭」不是同一件事。
- [ ] `degraded` 仍显示「降级」，SQL 禁用，文案仍是未就绪，而不是邀请用户去打开。

### 11.2 API 与状态

- [ ] `POST .../open` 与 `POST .../close` 返回 `404`，JSON `error.code` 为 `not_found`。
- [ ] `POST .../databases` 成功体 `status` 为 `ready`。
- [ ] 已有 `status=closed`（以及 `opening` / `closing` / `recovering`）且 `deleted_at` 为空的行，不经手动 open 即出现为 `ready`，并且 query 成功。
- [ ] `degraded` 不会被这次迁移改成 `ready`。
- [ ] `DELETE` 仍软删：状态 `deleted`（或清理完成前为 `deleting`）、写 `deleted_at`、清理该库平面 B 前缀，不清理用户文件前缀。

### 11.3 运行时

- [ ] `CloseIdle` 后 catalog 仍为 `ready`；下一次 query 成功，调用方无额外请求。
- [ ] 有活跃引用时，内部 `CloseDatabase` 仍拒绝强制关闭（现有 registry 测试保持）。
- [ ] 删除与缓存淘汰仍会调用 `Registry.CloseDatabase`。
- [ ] `max_open`、idle timeout、`Shutdown` 行为不因本功能改变。

### 11.4 工程

- [ ] `go test` 相关包通过；UI 单测不再引用 `databases.open` / `close`。
- [ ] 被删的 open/close 用例有 §11.2 / §11.3 的替代断言，而不是只删掉覆盖。
- [ ] JS SDK 类型与 `dist` 不再导出 `open` / `close`。
- [ ] §9 用户可见文档不再把打开/关闭写成使用步骤。

建议在实现 PR 里点名替换的现有测试：

| 现在 | 改成 |
|---|---|
| `api.TestPlan_OpenDatabasePromotesCreatingToReady` | 启动修复或读取兜底：残留 `creating`（descriptor 已在）变为 `ready`；无 descriptor 变为 `degraded` |
| `api.TestOpenDatabase_Success` / `TestCloseDatabase_Success` | 路由 `404 not_found` |
| `catalog.TestPlan_CreateDatabaseBecomesReady` | 保持，作为「创建即 ready」的回归 |

---

## 12. 明确不做 / Out of scope

- 不改 DuckLake catalog、Parquet、`DATA_PATH` 或 descriptor 的存储格式；不回写历史 descriptor 的 `status` 字段。
- 不删除 `CloseIdle`、缓存 LRU、`database.max_open`。
- 不做设置页，也不把空闲超时暴露成控制台开关。
- 不恢复 backups/restore。
- 不改云 Agent 工具列表。
- 不把 `degraded` 做成自动重试打开。
- 本文件所在 PR 不实现上述代码与文档修改。

---

## 13. 触点一览

```text
UI
  ui/src/pages/Databases.vue
  ui/src/lib/status.ts
  ui/src/services/{types.ts,http-api.ts,mock.js}
  ui/src/pages/*test.ts  ui/src/services/*test.ts  ui/src/test/*  ui/src/coverage-gaps.test.ts

HTTP
  remove        POST /v1/projects/:projectID/databases/:databaseID/open
  remove        POST /v1/projects/:projectID/databases/:databaseID/close
  keep          POST/GET/DELETE databases, query/execute/batch

Catalog
  sys_databases.status  closed|opening|closing|recovering → ready
  KEEP                  ready, creating, degraded, deleting, deleted

Registry（内部）
  keep  Acquire, Release, CloseIdle, Shutdown, CloseDatabase
  drop  仅 HTTP/UI/SDK 上的手动 close

SDK
  packages/js-sdk/src/databases.ts
  packages/js-sdk/dist/index.js
  packages/js-sdk/dist/index.d.ts
```

---

## 14. 与现有文档的关系

| 文档 | 关系 |
|---|---|
| `plan/planv2.0/proto-http.md` | 今天的 HTTP 契约。实现时删 open/close，并改四处重复的 §3.1 |
| `plan/planv2.0/databases-and-s3-plan.md` | 引擎与软删已按该计划落地。其中「生命周期含 open/close」被本计划取代 |
| `plan/planv2.0/db-ducklake-plan.md` | 句柄打开 = ATTACH / 拉 catalog，仍是 `Acquire` 的内部实现，不再对应一个用户按钮 |
| `internal/AGENTS.md` | 系统库禁止删除与写入、只读 SELECT，本计划不放宽 |
| `ui/AGENTS.md` | admin 项目隐藏写操作；打开/关闭本来就在 `v-if="!isAdmin"` 里，删掉后这条仍然成立 |

实现时以本文件的决策为准。planv1 状态机里的 `closing → closed` 视为未在当前生产路径落地的设计，不需要补实现。
