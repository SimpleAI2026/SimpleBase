# 系统库按 SQL 表展示

> **仓库**：SimpleBase（https://github.com/linkxzhou/SimpleBase）
> **状态**：**已实施**（方案原文保留；实现与本文件同一变更）
> **日期**：2026-10-09
> **对照**：`main` @ `4db7c1b`
> **范围**：系统库 `simplebase-system`（`kind=system`，项目 `sb-admin`）在控制台「数据库管理」中从「集合」改为「表」；补只读的表数据分页。普通项目里的 SQL 库一并补上「查看表数据」。KV 库不动。

## 0. 目标

管理员打开 admin 项目的系统库时，看到的是表，而不是集合。每张表给出表名和字段（名称、类型、是否可空）。点进一张表后，按页查看行。系统库继续只读：不能建表、加列、改行、删行、执行写 SQL。凭证、密钥、token、password hash 一类列的值不离开服务端。

普通项目的 SQL 库现在只能看表结构，没有行浏览。同一套只读分页接口和面板也给它们用。它们的写入口保持现状，单元格不做脱敏。

KV（`kind=kv`）仍不进 SQL 表结构分支，也不出现在用户库列表。

## 1. 现状

### 1.1 数据形态判定

`internal/catalog/reserved.go` 把形态收成两个常量：`collection` 与 `sql`。系统库常量是 `DatabaseKindSystem`（`"system"`），展示名 `simplebase-system`，项目 `ReservedSystemProjectID`（`sb-admin`）。

`internal/catalog/data_model.go`：

| 函数 | 现在的行为 | 对系统库的结果 |
| --- | --- | --- |
| `NormalizeDataModel` | 空字符串与 `collection` → `collection`；`sql` → `sql` | 创建用户库时用。系统库不走创建接口 |
| `EffectiveDataModel` | 列值精确等于 `sql` 才返回 `sql`，其余返回 `collection` | 系统行的列是 `collection`（见下），所以 API 返回 `collection` |
| `IsSQLDataModel` | `kind` 为 `system` 或 `kv` 时直接 `false`，即使列上写了 `sql` | 系统库不进表结构分支，也不被集合接口拒绝 |

注释写明了这个短路的意图：系统库和 KV「即使列上残留 sql，也不进入 SQL 表结构 / 拒绝集合的分支」。`internal/catalog/data_model_test.go` 的 `TestIsSQLDataModel` 把「系统库 + `data_model=sql`」断言成非 SQL。

系统行的列值从哪来：

- `internal/systemdb/migrate.go` v43 `sys_databases_data_model` 给 `sys_databases` 加 `data_model VARCHAR DEFAULT 'collection'`，并把空值刷成 `collection`。
- `internal/systemdb/bootstrap.go` 的 `backfillSystemRow` 插入的 `catalog.Database` 没有设置 `DataModel`。`internal/catalog/sql_repository.go` 在插入和扫描时把空字符串收成 `collection`。
- 因此现存系统行的存储值是 `collection`。单靠改测试或改一处 `if` 不够，API 与存储要一起对齐。

### 1.2 对外的 `data_model`

`internal/api/database_handler.go` 的 `toDatabaseResponse` 把 `catalog.EffectiveDataModel(db)` 写进 JSON 字段 `data_model`。列表和详情都走这里。

`ui/src/services/http-api.ts` 的 `toDatabaseItem` 只有 `data_model === 'sql'` 才映射成 `'sql'`，其它（含缺省）都是 `'collection'`。

admin 项目的列表本身只含系统库：`catalog.Service.ListDatabases` 在 `IsSystemProject` 时调用 `ListDatabasesByKind(..., DatabaseKindSystem)`。普通项目走 `ListDatabases`，条件是 `kind = user`，系统库和 KV 都不会出现。`GetDatabase` 还有一道：系统行只有在系统项目下才能取到，否则 `ErrNotFound`。

### 1.3 `GET /schema` 与写表结构

路由在 `internal/api/router.go`：

| 方法 | 路径 | 权限 | Handler |
| --- | --- | --- | --- |
| GET | `/v1/projects/:projectID/databases/:databaseID/schema` | `DatabaseRead` | `SchemaHandler.ListSchema` |
| POST | `.../schema/tables` | `DatabaseWrite` | `CreateTable` |
| POST | `.../schema/columns` | `DatabaseWrite` | `AddColumn` |

`SchemaHandler.ensureSQL`（`internal/api/schema_handler.go`）的顺序：

1. 写操作且实例 `writable=false` → `database.ErrWriterUnavailable`（503）。
2. 取 principal、project、`GetDatabase`。
3. **写操作**且 `IsSystemDatabase` → `catalog.ErrSystemProtected`（403 `system_database_protected`）。
4. `!IsSQLDataModel` → 400 `data_model_mismatch`，「只有 SQL 数据库可以管理表结构」。

系统库卡在第 4 步。`GET /schema` 的 `write=false`，第 3 步不触发，然后被第 4 步拒绝。`schema_handler_test.go` 只覆盖了系统库的 **POST** 列（期望 403），没有覆盖 GET。

`ListSchema` 本身已经是只读查询：`Acquire(..., ReadOnly)`，SQL 读 `information_schema.columns` ⋈ `information_schema.tables`，限定 `current_database()` / `current_schema()` / `BASE TABLE`，按 `table_name, ordinal_position` 排序，上限 5000 行。结果由 `groupSchemaRows` 收成 `{ tables: [{ name, columns: [{ name, type, nullable }] }] }`。

系统库的只读租约已经接好：`internal/api/adapter.go` 的 `sqlServiceAdapter.Acquire` 在 `IsSystemDatabase && mode == ReadOnly` 时返回 `systemLeaseAdapter`，查询打在 `systemdb.Store` 的常驻 `*sql.DB` 上，不经过用户库 registry（两边 `CacheDir` 不同，registry 会打开空 catalog）。`systemLeaseAdapter.Execute` / `Batch` 固定返回 `ErrSystemProtected`。`db_stats.go` 的注释已经写明行数统计走这条桥。

所以 GET 一旦放行，表和字段会从真实系统库读出来，不需要第二条连接。

### 1.4 集合接口与 SQL 接口对系统库的处理

`DataHandler.acquire`（`internal/api/data_handler.go`）只在 `IsSQLDataModel` 时返回 400 `data_model_mismatch`。系统库今天是 false，因此：

- `GET .../data/collections` 会在系统连接上查 `information_schema.tables`，把 `sys_*` 表名当成集合名返回。
- `GET .../data/collections/:collection` 执行 `SELECT id, data FROM "<name>" ORDER BY created_at DESC`。系统表没有这三列的文档形状，查询失败或形状不对。
- 四个写接口另有 `systemGuard`，`IsSystemDatabase` 即 `ErrSystemProtected`。实例只读时先 503。

`SQLHandler`（`internal/api/sql_handler.go`）：

- `POST .../query`：`sqlguard.Validate(..., ReadOnly)` 后 `Acquire(ReadOnly)`。系统库允许，且走 `systemLeaseAdapter`。结果原样序列化，**不脱敏**。
- `POST .../execute` 与 `POST .../batch`：系统库在执行前返回 `ErrSystemProtected`。实例只读时先 503。

`sqlguard` 只做首关键字、多语句、NUL、DuckLake 元数据标识符和危险指令。它不管列名。用户库里名叫 `password_hash` 的列必须还能被自己的主人读到，所以敏感列规则不能写进 `sqlguard`。

云 Agent 的 `readonly_sql`（`internal/api/cloudagent_access.go` 的 `ReadOnlyQuery`）走 `registry.Acquire`，不走 `systemLeaseAdapter`。按 `internal/AGENTS.md`，这条路径打开的是空 catalog，读不到真实系统表。本次不要把它改接到系统库上（见 §5）。

### 1.5 谁能看见系统库

权限已经在项目入口，不需要新权限位。

- `auth.CheckProjectAccess`：`role=user` 在系统项目上直接拒绝；`superadminl1`、`admin`、持 `ProjectAdmin` 的 API Key 在同租户内可以进。
- `auth.PermissionsForRole`：`admin` 只有 `DatabaseRead` 与 `LLMInvoke`，没有写权限。`superadminl1` 有 `DatabaseWrite`，所以写保护必须留在 handler（`ErrSystemProtected`），不能只靠角色。
- 前端 `stores/project.ts` 的 `isAdmin`（`projectId === 'sb-admin'`）是只读体验的唯一入口。真正的拒绝在后端。

### 1.6 控制台怎么选面板

`ui/src/pages/Databases.vue`：

```text
isSqlDatabase(record) := record.dataModel === 'sql'
展开行：SQL → SchemaPanel；否则 → DataTabs → CollectionPanel
```

桌面表和 `md` 以下的卡片是同一判断。admin 项目把 `readonly` 设为 `isAdmin`。

因此系统库今天展开的是 `CollectionPanel`：集合名列表、「查看数据」打开 `DocumentListModal`（文档行 `id` + `data`）。`CollectionPanel` 在 `readonly` 时把空态文案改成「暂无数据表」，并藏起「新建集合 / 新增文档」，但数据源仍是集合接口。

`SchemaPanel.vue` 已经具备只读开关：`readonly` 时不渲染「新建表」「添加列」两个表单。它只展示表名和列清单（一节一个 `section`，不是表格），没有「查看数据」。加载态是一行「加载表结构」。失败只 `toast`，区域落成空列表「还没有表」。

`ui/tests/Databases.test.ts` 里 admin 展开用例断言的是 `.coll-panel[data-readonly=1]`，因为 mock 列表项没有 `dataModel: 'sql'`。实施时这个断言要改到 SQL 面板上。

SQL 工作台 `SqlWorkModal` 在 `readonly` 时只留查询模式，文案是「系统库只读：仅支持 SELECT 查询」。这条入口保留。

列表上的类型徽标是 `v-if="!isAdmin"`，admin 行不显示「SQL / 集合」，只在操作列显示「受保护」。

### 1.7 加载态和表格对齐

`TableHead` / `TableCell` 已经默认 `text-center`（`plan/planv5.0/console-table-center-agents-entry-plan.md` 已落地）。长文本、标识符、JSON 用页面上的 `text-left` 盖过默认值。数据库展开单元格已有 `text-left`，避免对齐继承进面板。操作列是直接子级 `flex`，居中靠组件上的 `[&>.flex]:justify-center`，不要再写 `justify-end`。

`useLoadState`、`SbAsyncRegion`、`SbTableSkeleton` 的契约写在 `plan/planv5.0/console-loading-states-plan.md` §4。对照的 `main` @ `4db7c1b` 只合入了该计划（#31），这三个文件还不在树里。`SchemaPanel` 因此仍是文案加载。

分页组件 `TablePager` 是展示层：接收 `page` / `pageSize` / `total` / `pageCount`。`usePagination` 是对已经拿全的数组做客户端切片。系统表（日志、指标、用量）不能先拉全表再切片，行浏览要服务端分页，翻页时重新请求，`TablePager` 只负责展示和翻页事件。

### 1.8 系统表里和凭据有关的列

以下列来自 `internal/systemdb/migrate.go` 的建表语句，值不能出现在表浏览或系统库 `POST /query` 的响应里：

| 表 | 列 | 原因 |
| --- | --- | --- |
| `sys_api_keys` | `key_hash` | API Key 摘要 |
| `sys_users` | `password_hash` | argon2id 口令编码 |
| `sys_user_sessions` | `refresh_token_hash` | refresh 摘要 |
| `sys_user_sessions` | `access_jti` | access token 的 jti |
| `sys_llm_provider_creds` | `credentials_json` | 注释写明含 `api_key` 原文，现有厂商 API 返回时已脱敏 |
| `sys_llm_provider_configs` | `credential_ref` | 凭证引用 |

这些列**保留在表结构里**（管理员需要看到字段存在），行里的值换成空，并由响应标明该列已隐藏。UI 显示「已隐藏」，以便和真正的 SQL NULL 分开。

明确不按子串去猜。`token_input`、`token_output`、`max_tokens`、`prompt_tokens`、`completion_tokens`、`reasoning_tokens`、`request_id` 都是用量或关联 ID，继续原样返回。

`sys_llm_messages.content`、`sys_agent_messages.content`、`sys_cloud_agents.system_prompt`、`sys_agent_schedules.prompt` 是业务正文，不是凭据列。本次不隐藏。日志和审计仍然不得打印单元格。

`sys_settings_global.value_json` / `sys_settings_project.value_json` / `sys_log_events.fields_json` 结构不固定，没有稳定的「这是密钥」列名。本次不扫描 JSON 内部。这是残留风险，写在 §5。

## 2. 后端

### 2.1 对外 `data_model=sql`

改 `internal/catalog/data_model.go`，系统库的对外形态和 SQL 分支以 `kind` 为准，不看列上残留的 `collection`：

- `EffectiveDataModel`：`kind=system` 返回 `sql`。其它行维持「列值精确为 `sql` 才是 sql，否则 collection」。
- `IsSQLDataModel`：`kind=kv` 仍返回 `false`。`kind=system` 返回 `true`。用户库仍是 `EffectiveDataModel == sql`。

`toDatabaseResponse` 已经调用 `EffectiveDataModel`，列表和详情会变成 `data_model: "sql"`。`toDatabaseItem` 不用改映射规则。

存储对齐，避免管理员打开 `sys_databases` 时看到系统行仍是 `collection`：

- 新迁移（当前最后一条是 v43，下一条用 v44）：`UPDATE sys_databases SET data_model = 'sql' WHERE kind = 'system' AND (data_model IS NULL OR data_model <> 'sql')`。幂等，可重复执行。不改用户库和 KV 行。
- `bootstrap.go` 构造系统行时写入 `DataModel: catalog.DataModelSQL`，让首次插入就是 `sql`。已存在的行靠 v44 更新。

`NormalizeDataModel` 不动。用户创建库时省略字段仍是集合文档。没有创建 `kind=system` 的 API。

KV：`IsSQLDataModel` 对 `kind=kv` 保持 `false`。不改 KV 的 `EffectiveDataModel`（KV 不进用户库列表；列上即便残留 `sql` 也不进表结构接口）。

### 2.2 `GET /schema` 列出系统库的表和字段

放行条件就是 §2.1 的 `IsSQLDataModel`。`ListSchema` 的 SQL、只读租约、5000 行上限保持不变。系统库因此自动走 `systemLeaseAdapter`。

在 `groupSchemaRows` 之后做两件过滤和标注，只影响响应，不改查询：

1. 丢掉表名以 `__` 开头的行（与 `sqlguard` 拒绝的 `__ducklake_metadata_` 前缀同一类引擎内部表）。`sys_migration_versions` 和全部 `sys_*` 业务表保留。
2. 列名命中 §2.5 的敏感集合时，该列增加 `sensitive: true`。用 `omitempty`，用户 SQL 库的响应不带这个字段，现有客户端不受影响。

`POST /schema/tables` 与 `POST /schema/columns` 的判断顺序保持现状：实例只读 → 503；系统库 → 403 `system_database_protected`；非 SQL → 400。`superadminl1` 也是 403。不要因为 `IsSQLDataModel` 变成 true 就把系统库放进 DDL。

### 2.3 只读分页读行

新增只读接口，不复用「前端拼 `SELECT *` 再调 `POST /query`」：

- 拼 SQL 的标识符来自用户路径，必须先白名单。
- 敏感列要在 SQL 里就不读取，而不是读出来再抹掉。
- `POST /query` 仍是自由 SELECT，留给 SQL 工作台；表浏览需要稳定的 `total` 和列元数据。

```text
GET /v1/projects/:projectID/databases/:databaseID/schema/tables/:table/rows?limit=&offset=
权限：DatabaseRead（与 GET /schema 相同）
中间件：现有 projectContextMiddlewareEcho
```

挂在 `SchemaHandler` 上。`NewSchemaHandler` 增加现有的 `SQLLimits`（超时、`MaxQueryRows`），`app.go` 传入已经给 `SQLHandler` 的那份。不新增配置项，不改 `config.yaml` 结构。

处理步骤：

1. `ensureSQL(c, false)`。读路径不看 `writable`。系统库与用户 SQL 库都通过；集合库 400 `data_model_mismatch`。
2. `normalizeSchemaIdent` 校验 `:table`（与建表同一条 `^[A-Za-z][A-Za-z0-9_]{0,62}$`）。不合法 400 `invalid_request`。系统表名 `sys_databases` 这类符合该式。
3. `limit` 默认 50，最大 200，解析失败 400。`offset` 默认 0，最大 100000，非法或超出 400。与 `database_handler.parseQueryLimit` 的上限习惯一致，但默认用 50，避免一页拉 200 行宽表。
4. `Acquire(ReadOnly)`。系统库走现有桥，用户 SQL 库走 registry。
5. 用参数查询确认该名是当前库、当前 schema 的 `BASE TABLE`，并取出 `ordinal_position` 顺序的列名与类型。没有这张表 → 404 `table_not_found`（新的 API 错误码，走 `WriteError`）。表名以 `__` 开头 → 同样 404，内部表不当成业务表。
6. 列名再次过 `normalizeSchemaIdent`，然后只从这份白名单拼 SQL。标识符用现有 `quoteIdentifier`。
7. 系统库：敏感列投影成 `NULL AS "列名"`，**SELECT 列表里不出现该列的真实引用**。用户 SQL 库：投影全部真实列，不做替换。
8. 排序：在列里按这个顺序挑第一个存在的——`occurred_at`、`created_at`、`updated_at`、`started_at`（`DESC`），否则 `id`（`ASC`），否则 `ordinal_position` 第一列（`ASC`）。时间列之外再加 `, "id" ASC`（该列存在且不是主排序列时），让分页在并列时间戳上稳定。
9. `SELECT COUNT(*) FROM "表"` 得到 `total`，再 `SELECT <投影> FROM "表" ORDER BY ... LIMIT ? OFFSET ?`。`LIMIT`/`OFFSET` 用占位参数。两条语句都先过 `sqlguard.Validate(..., ReadOnly)`，作为拼接错误的保险。
10. 上下文超时用 `SQLLimits.QueryTimeout`，与 `POST /query` 相同。

响应：

```json
{
  "table": "sys_users",
  "columns": [
    {"name": "id", "type": "VARCHAR", "nullable": false},
    {"name": "password_hash", "type": "VARCHAR", "nullable": false, "sensitive": true}
  ],
  "rows": [["uuid", null]],
  "limit": 50,
  "offset": 0,
  "total": 1
}
```

`rows` 与 `columns` 对齐，沿用 `POST /query` 的列式数组，避免再定义一套 map 行。`sensitive: true` 只出现在系统库命中敏感集合的列上。

日志与审计：可以记表名、limit、offset、行数、耗时、错误码。不记单元格，不记拼接前的用户原文以外的值。敏感列的值根本不在 SELECT 里。

`DataHandler` 的集合路由不用改判断式。系统库变成 `IsSQLDataModel` 之后，`GET/POST .../data/collections` 以及文档读写都会 400 `data_model_mismatch`。写路径上的 `systemGuard` 保留，作为双保险。

### 2.4 堵住 `POST /query` 旁路

只改系统库的 `SQLHandler.Query`。用户 SQL 库的查询结果保持原样，这样用户自己的 `password_hash` 列仍可读。

两层一起做：

1. **执行前**：在 `sqlguard.Validate(ReadOnly)` 通过之后、`Acquire` 之前，若 `IsSystemDatabase`，用与 `sqlguard.hasDuckLakeMetadataIdentifier` 同类的扫描（跳过字符串字面量、识别双引号标识符）查找 §2.5 的列名。命中则 403 `system_column_redacted`，SQL 不执行。这样 `SELECT password_hash AS h`、`substr(password_hash, 1, 4)` 都进不了引擎。
2. **执行后**：结果列名命中敏感集合时，该单元格改成 JSON `null`，并在 `QueryResponse` 增加 `redacted_columns: ["password_hash"]`（`omitempty`，没有则不出现）。`SELECT * FROM sys_users` 的结果列名是真实列名，靠这一层抹掉。值会短暂进入进程内存；禁止把 `Rows` 打进日志。表浏览接口不依赖这一层。

`execute` / `batch` 继续 403，不增加新分支。

扫描放在 `internal/api`（例如 `system_redact.go`），供 schema 行投影、`ListSchema` 标注和 `Query` 共用一份列名集合。不放进 `sqlguard`，避免用户库被同一张名单挡住。

### 2.5 敏感列名单

一份精确列名集合，比较时转小写：

```text
password_hash
key_hash
refresh_token_hash
access_jti
credentials_json
credential_ref
```

后续迁移若新增密钥列，同一改动里把名字加进这个集合，并补一条「名单含该列、不含 `token_input`」的测试。

不隐藏整张表。`sys_llm_provider_creds` 仍可浏览，`credentials_json` 的值是 null 且 `sensitive: true`。

### 2.6 写路径与 KV

| 入口 | 系统库 | 用户 SQL 库 | KV / 集合库 |
| --- | --- | --- | --- |
| `GET /schema` | 200，表和字段 | 200，与现在相同 | 400 `data_model_mismatch` |
| `GET .../rows` | 200，敏感列为空 | 200，原值 | 400 |
| `POST /schema/tables`、`/columns` | 403；实例只读时 503 | 现有 DDL | 400；系统库不会落到这里 |
| `POST /query` | 只读 SELECT；敏感标识符 403；`SELECT *` 按列名抹掉 | 不变 | 不变（本来就可读，只要 sqlguard 通过） |
| `POST /execute`、`/batch` | 403 | 不变 | 系统库 403 |
| 集合与文档读写 | 400；写接口另有 403 | 400（已是 SQL） | 不变 |
| `kind=kv` | — | — | `IsSQLDataModel` 仍为 false，不出现在库列表 |

实例 `writable=false` 时，新的 GET 仍可用。写接口的 503 优先于系统库 403，与 `ensureSQL` 现在的顺序一致，已有只读实例测试不用改期望。

## 3. 前端

数据源以 API 的 `data_model === 'sql'` 为准。admin 项目的列表只有系统库，展开时再用 `projectStore.isAdmin` 兜住：即便某次响应缺了字段，也不要退回 `CollectionPanel`。普通项目仍只看 `dataModel`。

### 3.1 数据库列表

`Databases.vue` 的展开分支改为「admin 项目或 `isSqlDatabase` → `SchemaPanel`」。`DataTabs` / `CollectionPanel` 只留给普通项目的集合库。

admin 行的类型徽标改为「SQL」，保留操作列「受保护」和页头「只读且不可删除」。桌面和移动卡片一起改。新建数据库、删除、新建集合继续只在非 admin 出现。

SQL 工作台入口保留，`readonly` 仍绑定 `isAdmin`。

### 3.2 表面板

`SchemaPanel` 在拿到表结构后用表格，而不是一节一段的清单：

| 列 | 对齐 | 内容 |
| --- | --- | --- |
| 表名 | `text-left`，`sb-mono` | `table.name` |
| 字段 | `text-left` | `name type`，不可空加 `NOT NULL`，多项用逗号接在同一单元格，过长截断并加 `title` |
| 操作 | 默认居中 | 「查看数据」 |

`readonly` 时在表上方放一行说明：「系统库只读，不能新建表或修改字段」。新建表、添加列的两个表单继续放在 `v-if="!readonly"` 里，用户 SQL 库不受影响。敏感列在字段摘要里追加「已隐藏」，不展示值（结构接口本来就没有值）。

空表库用 `SbEmptyState`「还没有表」。这个文案只在请求结束后出现。

### 3.3 查看表数据

每张表的「查看数据」打开 `SbModal`（新组件，例如 `ui/src/components/databases/SchemaRowsModal.vue`）。标题是表名。系统库在标题旁标明「只读」。

弹窗内是 `Table`：表头为列名（`text-center`），单元格默认居中。`sb-mono` 标识符、截断的长文本和 JSON 列加 `text-left`。`sensitive` 列渲染「已隐藏」，不渲染空白。真正的 null 渲染成淡色 `NULL`。外层沿用 `Table` 自带的横向滚动，系统宽表不用另写 CSS 文件。

底部分页用 `TablePager`。`pageSize` 固定 50。`total` / `pageCount` 来自接口的 `total`。翻页换 `offset = (page - 1) * 50` 重新请求。弹窗打开时从第一页开始。换表时清空上一张表的行。

用户 SQL 库使用同一个弹窗和同一个接口，没有「已隐藏」列，也没有只读标题（写结构的入口在面板上，不在这个只读弹窗里）。弹窗不提供插入、更新、删除。

`services/types.ts` 用 `interface` 增加 `SchemaColumn.sensitive?: boolean` 和表行响应（`table`、`columns`、`rows: unknown[][]`、`limit`、`offset`、`total`）。`http-api.ts` 增加 `rows` 方法，snake/直出字段经映射函数转成该 interface，不把 raw 对象交给页面。新代码不写 `any`。`api-mock.ts` 补上这个方法的默认桩。

`packages/js-sdk` 本次不改。已有的 `schema()` 在系统库上会从 400 变为 200，这是兼容的放宽。SDK 不增加 rows 方法。

### 3.4 加载态

表列表和行表都走 `console-loading-states-plan.md` §4 的三件原语：

- `useLoadState`：160ms 内返回不闪骨架；失败写入 `error`；已有数据时刷新保持旧行（`refreshing`）。
- `SbAsyncRegion`：骨架 / 错误（`Alert` + 重试）/ 空态 / 内容四选一。空态不能在 `pending` 时出现。
- `SbTableSkeleton`：列数与最终表一致（表列表 3 列；行表等于本次 `columns.length`，未知时先 4 列）。

若实施时这三个文件仍未合入，按该计划 §4 的接口把它们和对应单测一起补上，**只**在本功能的表列表和行弹窗使用。不要在本改动里把数据库列表、云函数、日志等页面换成这套闸门。`useAsyncAction` 继续留给创建、删除的 toast。

失败时区域里看得到错误和重试。`toast` 可以保留作补充，但不能是唯一反馈。

## 4. 测试与验收

### 4.1 Go

不新增依赖，不改 `go.mod`。handler 用现有 fake service / fake lease，不连 DuckDB 或 S3。

| 用例 | 期望 |
| --- | --- |
| `EffectiveDataModel` / `IsSQLDataModel`：系统库列值为空、`collection`、`sql` | 两者都是 SQL |
| KV 列值为 `sql` | `IsSQLDataModel` 为 false |
| 用户库空列 | 仍是 collection |
| `GET /schema` 系统库 | 200，`Acquire` 模式为 `ReadOnly`，响应含表和列；`password_hash` 带 `sensitive: true`；`__` 前缀表被丢掉 |
| `POST /schema/tables` 与 `/columns` 系统库 | 403 `system_database_protected`（现有用例保留） |
| 实例 `writable=false` 时 POST | 503，顺序不变 |
| 集合库 GET schema 与 GET rows | 400 `data_model_mismatch` |
| GET rows：表名非法、表不存在、limit/offset 非法、offset 超上限 | 400 或 404，与 §2.3 一致 |
| GET rows 系统库 | 生成的 SQL 含 `NULL AS "password_hash"`，不含对 `password_hash` 列的读取；`sensitive: true`；limit/offset 是参数 |
| GET rows 用户 SQL 库 | SQL 读取真实列，无 `sensitive` |
| 系统库 `ListCollections` | 400 `data_model_mismatch` |
| 系统库文档写接口 | 仍 403 |
| `POST /query` 系统库，SQL 含 `password_hash`（含别名、函数参数、双引号） | 403 `system_column_redacted`，lease 未被调用 |
| `POST /query` 系统库，结果列名是敏感列（模拟 `SELECT *`） | 单元格为 null，`redacted_columns` 含该列 |
| `POST /query` 用户 SQL 库，选择 `password_hash` | 200，原值还在 |
| `POST /execute` 系统库 | 仍 403 |
| 迁移 v44 | 重复执行成功；只改 `kind=system` |

`go build ./internal/... ./cmd/...` 与相关包 `go test` 通过。本次不要求为了计划本身跑完全库；实施时按 `AGENTS.md` 跑 `go test ./...`。

### 4.2 UI Vitest

覆盖率阈值维持 `ui/vite.config.ts` 里的 statements / lines / functions / branches 各 95%。新的 vue 与 ts 都在统计范围内（`types.ts` 与 `src/components/ui/` 除外）。每个新函数都要有断言打到的路径，避免 functions 掉下去。

不桩 `Switch`。表用真实 `Table` / `TableHead` / `TableCell`，断言 `text-left` 只出现在表名、字段、长文本上。`SchemaPanel` 现有测试保留：用户 SQL 库仍能提交新建表和添加列。

要补的行为：

- admin 项目展开系统库（列表项 `dataModel: 'sql'`，以及缺省 `dataModel` 的兜底）渲染 schema 面板且 `readonly`，不渲染 `.coll-panel`，不出现「新建集合」。
- 面板加载中出现骨架，不出现「还没有表」；失败出现错误区；成功出现表名和 `id VARCHAR`。
- `readonly` 时没有「新建表」「添加列」，有只读说明。
- 「查看数据」请求 `limit=50&offset=0`，第二页请求 `offset=50`。敏感列显示「已隐藏」。空表显示空态。
- 普通项目 SQL 库同样能打开行弹窗，单元格是原值，新建表表单仍在。
- 普通项目集合库仍走集合面板。

`yarn test` 与 `yarn build` 通过。`yarn typecheck` 的既有 `*.vue` shim 问题不在本次解决。

### 4.3 验收标准

1. admin 项目中系统库的 API `data_model` 为 `sql`；徽标为 SQL；展开后是表，不是集合。
2. 表列表有表名和字段类型；点表可翻页看行；加载、空、错误三态分开。
3. 系统库没有新建表、加字段、改行、删行入口。`POST` 表结构、`execute`、`batch`、集合写入返回 403（实例只读时写接口 503）。
4. `password_hash`、`key_hash`、`refresh_token_hash`、`access_jti`、`credentials_json`、`credential_ref` 在行接口中值为 null 且标记敏感；SQL 工作台不能用别名把它们查出来。
5. `role=user` 与普通项目 API Key 仍然进不了 `sb-admin`。`admin` 与 `superadminl1` 可以看。
6. 普通项目的集合库和 KV 页行为与现在一致。普通项目 SQL 库可以看表数据，写结构入口还在。
7. Go 测试与 `ui` 的 `yarn test` / `yarn build` 通过，覆盖率不低于 95%。

## 5. 风险与不做的事

**风险**

- 系统库 `IsSQLDataModel` 变为 true 之后，任何只判断这个函数的调用都会把系统库当成 SQL。今天仓库里只有 `DataHandler.acquire` 和 `SchemaHandler.ensureSQL`。实施时再搜一次，避免把系统库放进用户库 DDL 或文档写入。
- `GET /schema` 与行接口必须走 `DataService.Acquire`，才能用上 `systemLeaseAdapter`。不要改成 `registry.Acquire`。
- `SELECT *` 的脱敏发生在结果列上，值会进入进程。执行前的标识符扫描挡住别名。两者都要有测试。扫描不是完整 SQL 解析器，和 `sqlguard` 一样可能被怪异的注释或拼接绕过；表浏览路径不把用户 SQL 送到引擎，旁路只存在于 SQL 工作台。
- `COUNT(*)` 加一页 `SELECT` 会打在系统 DuckLake 上。日志表和指标表可能慢，并受 `QueryTimeout` 约束。只在管理员打开某一张表时发生，不放进数据库列表的 `document_count` 刷新。列表上的「数据量」仍是现有的 `CountDatabaseRows`（最多 200 张表累加），本次不改它的策略。
- `value_json`、`fields_json` 里若被写进过密钥，列名脱敏看不到。那是写入方的问题，不在本接口用启发式去扫。
- 深分页 `OFFSET` 在大表上会变慢。`offset` 上限 100000 用来挡住失控请求，不保证最后一页很快。
- 加载态三原语若与另一个正在落地的加载态改动同时改 `SchemaPanel`，以那份实现的组件 API 为准，本功能只接闸门，不重写骨架样式。

**不做**

- 不实现代码（本文件只是计划）。
- 不让系统库可写，不提供行的插入、更新、删除，不开放 `POST /schema/tables` 与 `/columns`。
- 不改 KV 的判定、列表和 Key-Value 页。
- 不把 LLM / Agent 正文、system prompt、定时任务 prompt 当作凭据隐藏。
- 不把云 Agent `readonly_sql` 改接到系统库常驻连接。那条路径今天因 `CacheDir` 不同读到空库；接上真实连接就必须先接同一套脱敏，那是另一次改动。
- 不改 `packages/js-sdk` 的方法列表。
- 不新增依赖，不改 `go.mod`、`package.json`、`tsconfig.json`、`vite.config.ts`、`config.yaml` 的结构。
- 不在本改动里全站替换加载态，不改 `useAsyncAction` 的签名。
- 不编辑 `internal/web/dist`。
- 不调整 admin 项目的成员范围，不新增权限位。

## 6. 实施时会碰到的文件

| 文件 | 改动 |
| --- | --- |
| `internal/catalog/data_model.go`、`data_model_test.go` | 系统库对外为 SQL；KV 短路保留 |
| `internal/systemdb/migrate.go`、`bootstrap.go` 及迁移测试 | v44 回填；引导行写入 `sql` |
| `internal/api/schema_handler.go`、`schema_sql.go`、`router.go`、`app.go` | GET schema 标注敏感列；新的 rows 路由；构造函数带上 `SQLLimits` |
| `internal/api/system_redact.go`（新） | 列名集合、标识符扫描、结果抹除 |
| `internal/api/sql_handler.go`、`error.go` | 系统库查询两层脱敏；`system_column_redacted`、`table_not_found` |
| `internal/api/*_test.go` | §4.1 |
| `ui/src/pages/Databases.vue`、`components/databases/SchemaPanel.vue` | 面板选择、只读表列表 |
| `ui/src/components/databases/SchemaRowsModal.vue`（新） | 分页行 |
| `ui/src/services/types.ts`、`http-api.ts`、`test/api-mock.ts` | rows 契约与映射 |
| `ui/src/composables/useLoadState.ts`、`components/SbAsyncRegion.vue`、`components/SbTableSkeleton.vue` | 仅当加载态计划尚未落地时按该文补上 |
| `ui/tests/Databases.test.ts`、`SchemaPanel.test.ts`、行弹窗测试 | §4.2 |

`DataHandler` 的判断式可以不改，行为会随 `IsSQLDataModel` 一起变；补一条系统库集合列表变为 400 的测试即可。
