# 数据库默认就绪：去掉打开 / 关闭

> **仓库**：SimpleBase（https://github.com/linkxzhou/SimpleBase）。本计划只描述这个仓库里的控制台、HTTP 与 catalog，不涉及别的产品线。  
> **状态**：plan-only。本 PR 只新增本文，不改产品代码。  
> **日期**：2026-09-24  
> **Verified against**：`main` @ `1a1bf75`。下面的路径与行为都对过当前代码。  
> **关联**：同目录计划（中文、决策表、阶段、DoD、文件清单）。实现时以本文的锁定决策为准。

### 已锁定的产品决策

1. **不需要「打开 / 关闭」。** 数据库创建成功后就是可用的（`ready`）。用户不必先预热、也不必手动关掉。
2. **相关逻辑端到端清掉。** 控制台按钮、HTTP 路由、SDK、mock、状态文案、以及把 open/close 写成使用步骤的文档，都要删。内部空闲关句柄留下。
3. **本 PR 不实现。** 下面的阶段是下一次实现 PR 的顺序。

---

## 0. 一句话目标

创建之后列表显示「就绪」，SQL 与集合可以直接用。列表上没有「打开」「关闭」。进程仍可在空闲时释放本地连接；下一次查询自己重新打开，用户无感知。

---

## 1. 现状（已核对）

### 1.1 控制台

`ui/src/pages/Databases.vue`（非 admin 项目）：

| 项 | 现状 |
|---|---|
| 按钮 | 「打开」「关闭」 |
| 打开 | `openDb` → `api.databases.open`；成功 toast「已打开」 |
| 关闭 | `closeDb` → `api.databases.close`；成功 toast「`${name} 已关闭（数据保留）`」 |
| `isOpenable` | `status` 属于 `ready` \| `closed` \| `degraded` 时「打开」可点 |
| `isReady` | `status === 'ready'`。展开集合、SQL、新建集合都卡在 ready 上 |
| 关闭可点条件 | 仅 `isReady` |
| 测试 | `ui/src/pages/Databases.test.ts` 覆盖打开/关闭成功与失败 |

客户端：

| 文件 | 行为 |
|---|---|
| `ui/src/services/http-api.ts` | `POST .../databases/:id/open` 与 `.../close` |
| `ui/src/services/mock.js` | `open` 把 status 写成 `ready`；`close` 写成 `closed` |
| `ui/src/services/types.ts` | `DatabaseItem.status` 含 `opening` / `closing` / `closed` 等；`Api.databases` 有 `open` / `close` |
| `ui/src/lib/status.ts` | `closed` →「已关闭」，`opening` →「打开中」，`closing` →「关闭中」 |

没有单独的 i18n 文件，中文写在页面和 `status.ts` 里。admin 系统库用 `v-if="!isAdmin"` 已经藏起打开/关闭；「受保护」、只读 SQL、查看数据留下。

mock 的 `create` 返回 `creating`，要点「打开」才变成 `ready`。真实创建接口成功时已经是 `ready`（见 §1.3）。实现时 mock 必须改成创建即 `ready`，否则 `VITE_USE_MOCK=true` 仍像「建完不能用」。

### 1.2 HTTP

挂在 `internal/api/router.go`，权限都是 `DatabaseAdmin`（`database:admin`），与创建、删除相同。没有单独的 open scope。`writable = false` 时与其它写接口一样 `503 writer_unavailable`。

| 方法 | 路径 | Handler |
|---|---|---|
| POST | `/v1/projects/:projectID/databases/:databaseID/open` | `DatabaseHandler.OpenDatabase` |
| POST | `/v1/projects/:projectID/databases/:databaseID/close` | `DatabaseHandler.CloseDatabase` |

`OpenDatabase`（`internal/api/database_handler.go`）：`GetDatabase` → `Acquire(ReadWrite)` → `Release` → `SetDatabaseReady`（注释写明兼容历史卡在 `creating` 的库）→ 再读一次并返回数据库 JSON，200。

`CloseDatabase`：注释是「释放本地资源（关闭连接），不删除 S3 数据」。`GetDatabase` 校验归属后调用 `registry.CloseDatabase`，**204 无 body**。不改 catalog 的 `status`。

### 1.3 Catalog

`internal/catalog/model.go` 的 `DatabaseStatus` 九个值都在：

| 常量 | 值 |
|---|---|
| `DatabaseCreating` | `creating` |
| `DatabaseOpening` | `opening` |
| `DatabaseReady` | `ready` |
| `DatabaseClosing` | `closing` |
| `DatabaseClosed` | `closed` |
| `DatabaseDegraded` | `degraded` |
| `DatabaseDeleting` | `deleting` |
| `DatabaseDeleted` | `deleted` |
| `DatabaseRecovering` | `recovering` |

`sys_databases.status` 是 `VARCHAR`，没有 CHECK（`internal/systemdb/migrate.go`）。

读代码后的补充（实现时按这个，不要按「用户必须先打开」来做）：

- `catalog.Service.CreateDatabase` 在记录写成功后调用 `SetDatabaseReady`，返回值已是 `ready`。`creating` 只活在这次请求内部。open 把卡住的 `creating` 推到 `ready`，是崩溃窗口的补救，不是正常步骤。
- 生产路径没有把状态写成 `opening`、`closing` 或 `closed`。`closed` 出现在删除允许的来源状态里，以及 mock / 单测。HTTP close 不会把行写成 `closed`。
- `SetDatabaseReady` 的来源只有 `creating` / `opening` / `recovering`。`closed` 会变成 `ErrInvalidState` 并被 open handler 忽略，所以**今天的打开按钮也不能把 `closed` 变成 `ready`**。控制台仍因 `isReady` 禁用 SQL。
- 列表查询条件是 `deleted_at IS NULL`，不是 `status != closed`。软删会写 `deleted_at`。

### 1.4 运行时

`internal/database/registry`：

| 方法 | 行为 | 本计划 |
|---|---|---|
| `Acquire` | 没有句柄时打开；已有则复用 | **保留**。查询、集合、数据 API 都走它 |
| `CloseDatabase` | `active == 0` 时丢掉句柄；有引用则报错。不删 S3 | **保留方法**。删除流程和缓存淘汰还在用。去掉的是用户/HTTP 入口 |
| `CloseIdle` | 空闲且 `active == 0` 的句柄自动关掉 | **保留** |
| `Shutdown` | 进程退出时关句柄 | **保留** |

`validateAccess` 拒绝 `deleting` / `deleted` 和 `degraded`。不拒绝 `closed`。句柄被 `CloseIdle` 清掉后，下一次 `Acquire` 会重新打开。挡住「已关闭就不能查」的是控制台的 `isReady`，不是 registry。

云 Agent 工具是 `list_databases`、`list_collections`、`readonly_sql`，不调用 open/close。实现时不要改 Agent。

仓库里没有 OpenAPI 文件。契约在 `plan/planv2.0/proto-http.md`（§3.1 的 open/close 表在文件里重复四次，约 123、609、1116、1602 行）和 `plan/planv2.0/proto.http`。JS SDK 在 `packages/js-sdk`（`src/databases.ts` 与已提交的 `dist`）。

---

## 2. 决策表

| # | 决策 | 含义 |
|---|---|---|
| 1 | 用户侧没有打开/关闭 | 列表、HTTP、SDK、mock 都不再提供 |
| 2 | 创建成功即可用 | 对外状态是 `ready`。不要求预热 |
| 3 | 始终可用 ≠ 句柄永不关闭 | `CloseIdle`、缓存淘汰、`Shutdown`、`max_open` 都留着。下次请求 `Acquire` |
| 4 | 关闭 ≠ 删除 | 删除仍是软删：`deleting` → 内部 `CloseDatabase` → 清该库对象前缀 → `deleted` 且写入 `deleted_at`。close 的注释是不删 S3；去掉 close 之后，删除仍是唯一清数据的动作 |
| 5 | 公开路由一次删除 | 见 §4。不先留一个弃用版本 |
| 6 | `Registry.CloseDatabase` 不删 | 只从 handler / UI / SDK 上拿掉手动调用 |

---

## 3. 产品 / UX

心智用一句话就够，不要解释连接池：

> 新建数据库后即可查询、建集合。不需要打开或关闭。

### 3.1 拿掉

- `Databases.vue` 的「打开」「关闭」、tooltip、`openDb`、`closeDb`、`isOpenable`。
- toast：`已打开`、`已关闭（数据保留）`、`打开失败`、`关闭失败`。
- `status.ts` 里只为开关存在的文案：`已关闭`、`打开中`、`关闭中`。没有单独的「库已关闭」空态，不要新做一块说明。
- 页面上不再使用的 `RocketIcon` / `PowerIcon` import。

### 3.2 留下

| 操作 | 条件 |
|---|---|
| 刷新、新建数据库 | 非 admin |
| 查看数据 / SQL / 新建集合 | 仍要 `ready`。`creating`、`degraded`、`deleting` 的 tooltip 继续是「数据库未就绪」 |
| 删除 | `status !== 'deleting'`，确认后提交。文案不要把「数据保留」挪到删除上 |
| admin | 只读 SQL、查看数据、「受保护」。本来就没有打开/关闭 |

创建成功 toast 应让人看出可以直接用（状态为「就绪」）。mock `create` 与真实 API 对齐，直接返回 `ready`。

Dashboard 用同一套 `statusText`（`ui/src/pages/Dashboard.vue`）。`closed` 文案删掉之后，夹具不要再断言「已关闭」。

---

## 4. API 怎么删

**假设：** 仓库外没有需要单独弃用窗口的 open/close 客户端。依据：没有 OpenAPI；调用方是本仓库的控制台、mock 和 `packages/js-sdk`；云 Agent 不调用；真实 close 也不把「已关闭」写进 catalog。

**做法：一次卸掉路由，不先 deprecate。**

SPA 兜底只注册了 `GET /*`（`internal/web/web.go`）。未注册的 POST 走 Echo 404，`errorHandler` 写成 JSON，`error.code = not_found`。验收按 **404 + `not_found`**，不是 HTML，也不是 `410`。

若以后确认有仓库外客户端，再单独加一版 `410`。那不是这一次的默认。

| 入口 | 现在 | 之后 |
|---|---|---|
| `POST .../open` | `DatabaseAdmin`，200，数据库 JSON | 不挂载 → POST `404 not_found` |
| `POST .../close` | `DatabaseAdmin`，204 | 同上 |
| `packages/js-sdk` `databases.open` / `close` | `src/databases.ts`、`dist/index.js`、`dist/index.d.ts` | 从接口删除，并更新已提交的 dist |
| `http-api.ts` / `types.ts` / `mock.js` | 与 `Api` 同一套签名 | 三处一起删 |

留下：创建、列表、详情、删除，以及 `query` / `execute` / `batch`、集合与文档。

`DatabaseAdmin` 权限保留，创建和删除还要用。

handler 的 `DatabaseService` 上，只为 open/close 存在的 `Acquire`、`CloseDatabase`、`SetDatabaseReady` 可以从 **handler 接口** 去掉。SQL/数据用的是另一套 `Acquire`。`registry.Registry.CloseDatabase` 不动。`catalog.Service.SetDatabaseReady` 仍由创建成功路径调用。

---

## 5. 状态与 `closed` 迁移

| 状态 | 决定 | 理由 |
|---|---|---|
| `ready` | 保留，且是创建成功后的正常值 | 列表、SQL、集合都以它为可用 |
| `creating` | 保留，仅创建请求内部短暂出现 | 成功响应不应停在这里。进程在写入与 `SetDatabaseReady` 之间崩溃时，今天靠 open 补救；open 去掉后用 §5.1 的启动修复 |
| `degraded` | 保留，且不要自动改成 `ready` | 创建失败会标降级。`Acquire` 拒绝。这是故障，不是「关着」 |
| `deleting` / `deleted` | 保留 | 软删。`deleted` 带 `deleted_at`，列表因 `deleted_at IS NULL` 看不到 |
| `closed` | 对用户消失 | 生产不写，mock 在写。已有行改为 `ready` |
| `opening` / `closing` | 对用户消失 | `model.go` 有常量，生产不写。已有行改为 `ready` |
| `recovering` | 对用户消失 | 生产不写，备份/恢复路由已不在契约里。已有行改为 `ready`。本计划不恢复备份 |

删除与关闭必须在测试里分开：

- 即将删除的 close：不写 `deleted_at`，不删对象，不把状态改成 `deleting` / `deleted`。
- 删除：来源状态在迁移后仍包括 `creating` / `ready` / `degraded`。迁移完成前若还见到 `opening` / `closed` / `recovering`，删除也要能接受，避免旧行删不掉。`deleted` 不能再变回可用库。

### 5.1 怎么把旧行变成可用

两步都做：

1. **启动时一次幂等更新**（catalog 启动或新的 migration version）：

   ```sql
   UPDATE sys_databases
   SET status = 'ready', updated_at = CURRENT_TIMESTAMP
   WHERE deleted_at IS NULL
     AND status IN ('closed', 'opening', 'closing', 'recovering');
   ```

   不改 `degraded` / `deleting` / `deleted`。不改对象存储。

2. **读取兜底：** `GetDatabase` / `ListDatabases` 若仍见到这四个值，响应按 `ready` 返回并写回。这样不依赖已经不存在的 open。

3. **卡住的 `creating`：** 不要在每次 list 上把「正在创建」无条件改成 `ready`。替换 open 补救的规则是进程启动时处理残留行：没有进行中的创建时，descriptor 存储未启用或对象已存在 → `SetDatabaseReady`；启用了对象存储但对象不存在 → `SetDatabaseDegraded`。正常 `CreateDatabase` 返回前已经是 `ready`，这条路径保持。

`SetDatabaseReady` 今天的来源不含 `closed`。迁移用上面的 SQL 直接写 `ready`。若读取兜底想复用该函数，来源要加上 `closed` / `opening` / `closing` / `recovering`，并且仍然不要从 `degraded` 转出。

---

## 6. 运行时：留下空闲关闭，去掉手动关闭

用户不控制连接池。

必须留在内部、并写成测试的行为：`CloseIdle`（或缓存淘汰）之后 catalog 仍是 `ready`；下一次 `query` / `execute` / 集合读通过 `Acquire` 重新打开并成功。失败时应是存储或引擎错误，而不是「请先打开数据库」。

`degraded` 的 `Acquire` 拒绝保持不变。

`internal/database/cache/evict.go` 开头「只淘汰已关闭」容易读成产品状态 `closed`。注释改成「只淘汰 registry 里没有活跃引用的库」。淘汰逻辑不变：先 `CloseDatabase`，再删缓存目录。

---

## 7. 前端清理清单

| 文件 | 动作 |
|---|---|
| `ui/src/pages/Databases.vue` | 删除打开/关闭、`isOpenable`、`openDb`、`closeDb` |
| `ui/src/pages/Databases.test.ts` | 不再期望「打开」「关闭」和「已关闭」 |
| `ui/src/pages/pages-coverage.test.ts` | 删除 `isOpenable({ status: 'closed' })` |
| `ui/src/pages/interactions.test.ts` | 用例里的 open/close 点击改为 SQL / 集合 / 删除 |
| `ui/src/pages/Dashboard.test.ts` | `closedDb` 若只为徽章，改成仍存在的状态 |
| `ui/src/coverage-gaps.test.ts` | 去掉 open/close 失败分支 |
| `ui/src/lib/status.ts` 与 `status.test.ts` | 去掉 `opening` / `closing` / `closed` / `recovering` 的用户文案 |
| `ui/src/services/types.ts` | 状态联合与 `open` / `close` 方法删除。注释改为：成功创建对 UI 是 `ready` |
| `ui/src/services/http-api.ts` 与 `http-api.test.ts` | 删除 `/open`、`/close` |
| `ui/src/services/mock.js` 与 `mock.test.ts` | `create` 返回 `ready`；删除 `open` / `close` |
| `ui/src/test/api-mock.ts` | 去掉 `databases.open` / `close` |
| `ui/src/test/helpers.ts` | `closedDb` 改为 `degraded` 或 `creating`，给「未就绪」用例用 |

---

## 8. 后端与 SDK 清理清单

| 文件 | 动作 |
|---|---|
| `internal/api/router.go` | 删除 open、close 两行 |
| `internal/api/database_handler.go` | 删除 `OpenDatabase`、`CloseDatabase`；收窄 handler 的 `DatabaseService` |
| `internal/api/adapter.go` | 删除只服务这两个 handler 的包装。删除流程仍把 `registry.CloseDatabase` 传给 closer |
| `internal/api/database_handler_test.go` | 删掉 open/close 成功用例和 `TestPlan_OpenDatabasePromotesCreatingToReady`。改为：创建 201 的 `status` 为 `ready`；POST open/close 为 `404` 且 `error.code=not_found` |
| `internal/api/handler_branches_test.go` | 从只读 / 未找到矩阵去掉 open/close |
| `internal/api/adapters_test.go` | 不再把 handler 的 `CloseDatabase` 当公开行为 |
| `internal/catalog/service.go` | §5.1 迁移与读取兜底；收紧删除的来源状态。创建路径继续 `SetDatabaseReady` |
| `internal/catalog/model.go` | 常量可留到迁移落地再删。注释不要再描述 `ready → closing → closed` 为现行状态机 |
| `internal/catalog/*_test.go` | `closed` 行读出来是 `ready` 且能查询。保留 deleting / deleted / degraded 用例 |
| `internal/database/registry` | **不**删 `CloseDatabase` / `CloseIdle` / `Shutdown`。补：idle 之后再次 `Acquire` 成功，且不改 catalog status |
| `internal/database/cache/evict.go` | 只改注释，见 §6 |
| `packages/js-sdk/src/databases.ts` 与 `dist/*` | 删除 `open` / `close`。`dist` 已入库，必须一起改 |
| `internal/cloudagent` | 不改 |

`Operation.Kind` 注释里的 `"open" | "close"` 没有对应的生产审计写入。不必回填旧审计行；新代码不要再写这两种 kind。

---

## 9. 文档清单（实现 PR 再改，本 PR 不改）

用户能读到、必须去掉「先打开再使用」的：

- [ ] `docs/database/index.md`：「创建 / 打开 / 关闭数据库」
- [ ] `docs/sdk/database-sql.md`：`sb.databases.open` / `close`
- [ ] `README.md` API 表里的 open/close 两行
- [ ] `internal/README.md`：handler 列表，以及 `creating→opening→ready→closing→closed` 那句
- [ ] `plan/planv2.0/proto-http.md`：四处 §3.1，以及「create/open/close/delete」
- [ ] `plan/planv2.0/proto.http`：打开数据库 / 关闭数据库示例
- [ ] `plan/planv2.0/js-sdk-plan.md`：`sb.databases.open / close`

历史计划只加一行「open/close 产品面已废弃，见本文」，不整篇重写：

- [ ] `plan/planv2.0/databases-and-s3-plan.md`（生命周期仍写了 open/close；`TestPlan_OpenDatabasePromotesCreatingToReady` 改成启动修复的测试名）
- [ ] `plan/planv2.0/ui-plan-v2.md`（九态与 open/close）
- [ ] `plan/planv1.0/plan.md`、`plan3.md`、`plan5.md`、`plan7.md` 里把 open/close 写成用户步骤的段落

`docs/ops/deployment.md` 里的 `max_open`、冷启动、进程退出时关闭连接是运维描述，保留。不要改成「用户可以关闭数据库」。

---

## 10. 阶段、风险、回滚

不要拆成两个已发布版本。

| 若只做这一步 | 会发生什么 |
|---|---|
| 只先改 UI | 历史 `closed` 与 mock 的 `creating` 仍让 SQL 禁用，按钮又没了 |
| 只先删 API | 旧页面 toast「打开失败 / 关闭失败」 |
| 先 deprecate 一个版本 | 没有需要窗口的外部客户端，close 也不落库 |

**一个实现 PR，内部按这个顺序：**

1. **Phase A — 旧行先可用。** §5.1 迁移、读取兜底、启动时处理残留 `creating`。测试：`status=closed` 的行列表为 `ready`，query 成功，且没有被标成已删除。
2. **Phase B — 同时拿掉公开表面。** 路由、handler、SDK、`http-api`、mock、`Databases.vue`、相关测试一起删。
3. **Phase C — 文档。** §9。可以跟 B 同一 PR。

### 风险

| 风险 | 处理 |
|---|---|
| 仓库外仍 POST open/close | JSON `404 not_found`。默认接受 |
| 残留 `creating` 被标成 `ready` 但对象没写上 | 启动修复按 §5.1：无对象则 `degraded` |
| `degraded` 被迁成 `ready` | 禁止 |
| 删除被做成「只关连接」 | 测试锁定 `deleted_at` 与对象前缀清理 |
| 只删测试、不补行为 | 用「创建即 ready」「旧 closed 可查」「idle 后查询成功」「POST open/close 为 404」替换 |
| `proto-http.md` 只改第一处 | 同一张表有四份 |
| SDK 只改 `src` | `dist` 已提交，会继续请求 `/open` |

### 回滚

- 代码：revert 实现 PR。
- 数据：`closed|opening|closing|recovering → ready` 不需要向下迁移。这些值没有单独的存储差异；revert 之后行留在 `ready` 仍然符合本决策。
- 不改对象格式，没有存储回滚。

---

## 11. 验收（DoD）

### 产品

- [ ] 数据库列表没有「打开」「关闭」，tooltip 和 toast 里也没有。
- [ ] 新建成功后列表为「就绪」，不经 open 即可 SQL 查询，并可新建集合。
- [ ] admin 系统库仍是只读 SQL + 查看数据 +「受保护」。
- [ ] 删除仍要确认；成功后走软删。与已经去掉的关闭不是同一件事。
- [ ] `degraded` 仍显示「降级」，SQL 禁用，文案是未就绪。

### API 与状态

- [ ] `POST .../open` 与 `POST .../close` 为 `404`，`error.code` 为 `not_found`。
- [ ] `POST .../databases` 成功体的 `status` 为 `ready`。
- [ ] 已有 `closed` / `opening` / `closing` / `recovering` 且 `deleted_at` 为空的行，不经手动 open 即变为 `ready`，并且 query 成功。
- [ ] `degraded` 不会被这次迁移改成 `ready`。
- [ ] `DELETE` 仍软删：`deleted`（清理完成前可为 `deleting`）、写入 `deleted_at`、清理该库对象前缀。close 不再存在，也不能靠别的按钮只关连接就声称删库。

### 运行时

- [ ] `CloseIdle` 后 catalog 仍为 `ready`；下一次 query 成功，调用方没有额外的 open 请求。
- [ ] 有活跃引用时，内部 `CloseDatabase` 仍拒绝强制关闭。
- [ ] 删除与缓存淘汰仍调用 `Registry.CloseDatabase`。
- [ ] `max_open`、idle timeout、`Shutdown` 不因本功能改变。

### 工程

- [ ] 相关 `go test` 与 UI 单测通过；UI 不再引用 `databases.open` / `close`。
- [ ] 被删用例有上面的替代断言。
- [ ] JS SDK 类型与 `dist` 不再导出 `open` / `close`。
- [ ] §9 里用户可见文档不再把打开/关闭写成使用步骤。

| 现有测试 | 替换 |
|---|---|
| `api.TestPlan_OpenDatabasePromotesCreatingToReady` | 启动修复：残留 `creating` 且对象已在 → `ready`；对象缺失 → `degraded` |
| `api.TestOpenDatabase_Success` / `TestCloseDatabase_Success` | 路由 `404 not_found` |
| `catalog.TestPlan_CreateDatabaseBecomesReady` | 保留，作为「创建即 ready」 |
| `ui/src/pages/Databases.test.ts` 的打开/关闭 | 断言按钮不存在，创建后 SQL 可用 |

---

## 12. 不做

- 不改数据库对象在存储上的格式，不回写历史 descriptor 的 `status`。
- 不删除 `CloseIdle`、缓存淘汰、`max_open`。
- 不做设置页，也不把空闲超时做成控制台开关。
- 不恢复备份/恢复接口。
- 不改云 Agent 工具。
- 不把 `degraded` 自动重试打开。
- 本 PR 不实现 §7–§9。

---

## 13. 文件与 API 触点

```text
UI
  ui/src/pages/Databases.vue
  ui/src/pages/Databases.test.ts
  ui/src/lib/status.ts
  ui/src/services/types.ts
  ui/src/services/http-api.ts
  ui/src/services/mock.js
  以及 §7 列出的测试夹具

HTTP  （卸掉）
  POST /v1/projects/:projectID/databases/:databaseID/open
  POST /v1/projects/:projectID/databases/:databaseID/close

HTTP  （保留）
  POST/GET/DELETE  .../databases
  POST             .../query | execute | batch

Catalog
  closed | opening | closing | recovering  →  ready
  保留 ready, creating, degraded, deleting, deleted

Registry（内部，保留）
  Acquire, Release, CloseIdle, Shutdown, CloseDatabase

SDK
  packages/js-sdk/src/databases.ts
  packages/js-sdk/dist/index.js
  packages/js-sdk/dist/index.d.ts
```
