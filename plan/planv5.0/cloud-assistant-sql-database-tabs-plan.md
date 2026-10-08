# 云助手、SQL / 集合数据库与页签居中

> **仓库**：SimpleBase（https://github.com/linkxzhou/SimpleBase）  
> **状态**：本分支同时落地计划与实现。  
> **日期**：2026-10-08  
> **Verified against**：`main` @ `be33ef5`。路径与行为按该提交核对。  
> **关联**：[`plan/planv3.0/database-always-open-plan.md`](../planv3.0/database-always-open-plan.md)（数据库创建后即 `ready`，本计划不恢复打开 / 关闭）。

## 0. 目标

1. 控制台目前叫「云 Agent」的功能改名为 **云助手**，并成为侧栏 **工作台** 分组的第一项。路由、权限、对话与工具行为不变。
2. **数据库管理** 支持两种数据形态，创建时选定，之后不改：
   - **SQL**：关系表。创建时可以提交初始化 SQL（例如 `CREATE TABLE`），之后在页面上建表、加列（`ALTER TABLE ... ADD COLUMN`）。
   - **集合文档**：维持现在的集合 / 文档 API 与页面。
3. 全站页签标题文字居中。改共享 `TabsTrigger`，横向和纵向都成立；设置弹窗、数据库页、文档站等现有用法不把整条页签栏重新排到页面正中。

DuckLake / S3 对象布局不改。两种形态都是同一套用户库文件，差别只在 catalog 标记和允许的 API / 页面。

## 1. 现状（已核对）

### 1.1 云 Agent 在导航里的位置

`ui/src/components/NavMenu.vue` 的分组是：

| 分组 | 项 |
| --- | --- |
| 工作台 | `dashboard`（监控大盘） |
| 数据 | `databases`、`key-value`、`s3` |
| 自动化 | `gofunctions`、`cron-jobs`、`sandboxes`、`agents` |
| 运维 | `logs`、`users` |

展示名来自 `ui/src/router/index.ts` 的 `meta.title`，`agents` 现为「云 Agent」。路径仍是 `/console/agents`，`/llm` 重定向到该路由。首页、监控大盘资源行、文档站模块标题也使用「云 Agent」。

会话默认标题写在 `AgentManager.vue`（`'云 Agent'`）。`internal/systemdb/agents.go` 在用户发出第一条消息时，若标题是 `New thread` 或 `云 Agent`，会改成消息摘要。

模型系统提示仍是英文 `SimpleBase Cloud Agent`（`internal/cloudagent/modules.go`）。这是模型行为，不是控制台文案，本次不改。

### 1.2 数据库、集合与 SQL

- Catalog 行在 `sys_databases`（`internal/systemdb/migrate.go` v3）。`kind` 只有 `user` / `system` / `kv`，表示访问类别，不是表形态。
- `catalog.Service.CreateDatabase` 写 catalog（可选 S3 descriptor）后立刻 `SetDatabaseReady`。没有「打开 / 关闭」。存储前缀由内部 UUID 生成，不拼用户输入。
- 集合文档（`internal/api/data_handler.go`）把集合建成表 `(id VARCHAR, data VARCHAR, created_at VARCHAR)`，文档是 JSON 字符串。列表走 `information_schema.tables`。
- SQL 控制台走 `POST .../query|execute|batch`，语句边界在 `database/sqlguard`：单条语句、拒绝 `ATTACH` / `COPY` / `SET` 等。`WriteAllowed` 允许普通 DDL / DML，因此今天任何用户库都能在 SQL 工作台里 `CREATE TABLE`。
- 控制台 `Databases.vue` 只有名称字段。展开行只有「集合文档」页签。没有初始化 SQL，也没有表结构页。
- JS SDK `databases.create({ name })` 只传名称。Go SDK `CreateDatabase(ctx, name)` 同样。
- 系统库 `kind=system` 只读；admin 项目仍是「查看数据 + 只读 SQL」。KV 是项目级独立资源，不出现在用户库列表。

### 1.3 页签

共享组件是 `ui/src/components/ui/tabs/TabsTrigger.vue`。横向已经 `justify-center`，纵向额外有 `group-data-[orientation=vertical]/tabs:justify-start`，文字靠左。

调用点（均横向）：

| 位置 | 列表对齐 | 触发器额外 class |
| --- | --- | --- |
| `SettingsModal.vue` | `w-full justify-start`（页签组从左起） | 无 |
| `DocsWiki.vue` | `justify-start` | `flex-none px-4` 等，宽度随文字 |
| `DataTabs.vue` | 默认 | 无 |
| `KvPanel.vue` / `KvApiPanel.vue` | 默认 | `px-3` |

没有页面把 `Tabs` 设成 `orientation="vertical"`。文档站和设置的 `justify-start` 作用在 **列表** 上，表示一组页签靠左排列，不是单个标题靠左。

## 2. 决策

| # | 决策 | 含义 |
| --- | --- | --- |
| 1 | 改名只动用户可见中文 | 路由名 `agents`、路径 `/console/agents`、API、权限、英文系统提示保持不变 |
| 2 | 云助手是工作台第一项 | `names: ['agents', 'dashboard']`。自动化组不再包含 `agents` |
| 3 | 数据形态是用户库的创建时属性 | 新列 `sys_databases.data_model`：`collection` 或 `sql`。不占用 `kind` |
| 4 | 省略或空值等于 `collection` | 旧客户端只传 `name` 时行为与现在一致 |
| 5 | 已有用户库默认集合文档 | 迁移把空值写成 `collection`。不改 DuckLake 文件，不改状态机，库仍然创建即 `ready` |
| 6 | 系统库与 KV 不参与这两种形态 | `IsSQLDataModel` 对 `kind=system` / `kind=kv` 恒为 false。admin 页面保持只读 SQL + 查看数据 |
| 7 | 初始化 SQL 只属于 SQL 库 | 服务端拆句、逐句 `sqlguard.Validate(WriteAllowed)`，首关键字只允许 `CREATE` / `ALTER` / `INSERT`。校验失败不建库；执行失败则删除刚建的库（名称可重试） |
| 8 | 表结构用结构化接口，不把任意 SQL 再包一层 | `GET /schema`、`POST /schema/tables`、`POST /schema/columns`。标识符走与集合名相同的正则，类型走白名单。SQL 工作台对两种库都保留 |
| 9 | 集合 API 拒绝 SQL 库 | 在 `DataHandler.acquire` 一处拒绝，避免把关系表列成集合。集合库行为不变 |
| 10 | 页签文字居中落在 `TabsTrigger` | 去掉纵向 `justify-start`，加上 `text-center`。不改设置 / 文档站列表上的 `justify-start` |
| 11 | 不做形态互转、删表、改列类型 | 见 §7 |

## 3. Catalog 与迁移

### 3.1 列

`internal/systemdb/migrate.go` 增加 **v43** `sys_databases_data_model`（v3 的 `CREATE TABLE` 已在现网执行过，不回溯修改）：

```sql
ALTER TABLE sys_databases ADD COLUMN data_model VARCHAR DEFAULT 'collection';
UPDATE sys_databases SET data_model = 'collection'
WHERE data_model IS NULL OR data_model = '';
```

`catalog.Database` 增加 `DataModel`。读写 SQL 都带上这一列。仓库插入时空字符串写成 `collection`。

直接自建 `sys_databases` 的测试 schema（`catalog/schema_test.go`、`registry/always_open_test.go`、`kv/sweeper_test.go`）补上同名列和默认值，否则 `SELECT` 对不齐。

### 3.2 规范化

| 输入 | 结果 |
| --- | --- |
| 空、`collection`（忽略大小写与空白） | `collection` |
| `sql` | `sql` |
| 其他 | `400 data_model_invalid`，不写 catalog |

`EffectiveDataModel`：不是精确的 `sql` 就当作 `collection`（兼容迁移前的空值）。  
`IsSQLDataModel`：`EffectiveDataModel == sql` 且 `kind` 不是 `system` / `kv`。

S3 descriptor 不增加该字段。对象键算法不变。

## 4. API

### 4.1 创建

`POST /v1/projects/:projectID/databases`（仍要 `database:admin`，仍检查 `writable`）：

```json
{ "name": "shop", "data_model": "sql", "init_sql": "CREATE TABLE users (id INTEGER, email VARCHAR)" }
```

| 字段 | 规则 |
| --- | --- |
| `name` | 与现在相同 |
| `data_model` | 可选，默认 `collection` |
| `init_sql` | 可选。非空时必须是 SQL 库。总长度 ≤ 64KiB，有效语句 ≤ 100 条 |

拆句在 `sqlguard.SplitStatements`（字符串、标识符、注释里的分号不算分隔）。`sqlguard` 仍拒绝多语句进入 `/query` 与 `/execute`；只有创建接口接受这一段脚本。

允许的首关键字：`CREATE`、`ALTER`、`INSERT`。`DROP` / `UPDATE` / `DELETE` / `SELECT` 拒绝。`ATTACH`、`CREATE SECRET` 等仍由 `sqlguard` 拒绝。

执行使用与 `/execute` 相同的 `Acquire(ReadWrite)`。任一条失败：`DeleteDatabase` 回滚 catalog 与该库对象前缀，HTTP 400 `init_sql_failed`（带语句序号和引擎错误，供提交者排错）。服务端日志不记录 SQL 原文。引擎未注入时，非空 `init_sql` 在建库前返回 `503 writer_unavailable`。

响应增加 `data_model`。列表和详情同样返回。不返回 `storage_prefix`。

### 4.2 表结构（仅 `data_model=sql` 的用户库）

| 方法 | 路径 | 权限 |
| --- | --- | --- |
| GET | `/v1/projects/:projectID/databases/:databaseID/schema` | `database:read` |
| POST | `.../schema/tables` | `database:write` |
| POST | `.../schema/columns` | `database:write` |

`POST .../tables`：

```json
{ "name": "users", "columns": [{ "name": "id", "type": "INTEGER", "nullable": true }] }
```

生成 `CREATE TABLE "users" ("id" INTEGER)`。`nullable: false` 时加 `NOT NULL`。省略 `nullable` 视为可空。至少一列，最多 32 列。

`POST .../columns`：

```json
{ "table": "users", "name": "email", "type": "VARCHAR" }
```

生成 `ALTER TABLE "users" ADD COLUMN "email" VARCHAR`。

标识符：`^[A-Za-z][A-Za-z0-9_]{0,62}$`（与集合名相同）。类型白名单：`BOOLEAN`、`TINYINT`、`SMALLINT`、`INTEGER`、`BIGINT`、`UTINYINT`、`USMALLINT`、`UINTEGER`、`UBIGINT`、`FLOAT`、`DOUBLE`、`DECIMAL`、`VARCHAR`、`DATE`、`TIME`、`TIMESTAMP`、`BLOB`、`JSON`、`UUID`（大小写不敏感，生成时用大写）。

写路径：`writable == false` → 503；`kind=system` → 403 `system_database_protected`；非 SQL 用户库 → 400 `data_model_mismatch`。

`GET` 读 `information_schema.columns`，只含当前库当前 schema 的 `BASE TABLE`，按表名与 `ordinal_position` 分组。

### 4.3 集合

集合六个路由不变。`acquire` 发现 `IsSQLDataModel` 时返回 400，文案为「SQL 数据库不支持集合与文档」。`data_model` 为空或 `collection` 的用户库、以及系统库的只读查看，仍走原 SQL。

### 4.4 SDK

- JS：`databases.create` 增加可选 `data_model`、`init_sql`；增加 `schema` / `createTable` / `addColumn`。`DatabaseInfo.data_model` 可选。
- Go：保留 `CreateDatabase(ctx, name)`；新增 `CreateDatabaseWith`；`DatabaseInfo.DataModel`；新增 schema 三个方法。

## 5. 控制台

### 5.1 云助手

- `meta.title`、侧栏、首页、监控大盘资源名、文档站模块标题、`ui/README.md`：云 Agent → 云助手。
- 工作台顺序：云助手、监控大盘。
- 新会话默认标题改为「云助手」。摘要改写同时识别旧标题「云 Agent」，已有会话仍会被第一条用户消息替换。

### 5.2 数据库页

新建弹窗：

- 名称（规则不变）。
- 类型：集合文档（默认） / SQL 数据。
- SQL 时显示初始化 SQL 文本框，可留空。

列表在名称旁标「集合」或「SQL」（admin 项目不标，避免把系统库说成集合产品）。

| 形态 | 操作 |
| --- | --- |
| 集合 | 与现在相同：SQL 工作台、新建集合、展开「集合文档」 |
| SQL | SQL 工作台保留；隐藏新建集合；展开为表结构面板 |
| admin | 不新增类型选择。展开仍是只读集合视图 + 只读 SQL |

表结构面板：列出表和列；新建表（表名 + 一列或多列，类型用固定枚举）；向已有表添加列。成功后重新加载 schema。只读时只展示，不显示表单。

页面用到的列类型是白名单的子集：`INTEGER`、`BIGINT`、`DOUBLE`、`VARCHAR`、`BOOLEAN`、`TIMESTAMP`、`DATE`、`JSON`。API 仍接受完整白名单，供 SDK 使用。

### 5.3 页签

`TabsTrigger` 在两种方向都使用 `justify-center` 与 `text-center`，并保留纵向 `w-full`。设置弹窗和文档站列表继续 `justify-start`：一组页签从左侧排，每个标题在自己的按钮里居中。文档站 `flex-none` 的页签宽度仍随文字，居中在内边距里看不出位移。

## 6. 测试

| 层 | 覆盖 |
| --- | --- |
| `sqlguard` | `SplitStatements`：字符串 / 注释中的分号、尾部分号、空脚本。原有「单条语句」用例必须仍通过 |
| catalog | 默认 `collection`、`SQL` 大小写规范化、非法值、读回 |
| systemdb | v43 之后插入且未写 `data_model` 的行是 `collection`；迁移条数与定义一致 |
| API | 带 init SQL 创建并执行每条语句；集合库带 init SQL 被拒绝且不建库；非法语句建库前失败；执行失败会删除库；表结构的查询 / 建表 / 加列；集合库与系统库拒绝表结构写；SQL 库拒绝集合列表；集合库创建集合仍成功 |
| UI | 云助手位于工作台第一项且文案正确；创建 SQL 库提交 `initSql`；集合库仍能新建集合；SQL 库展开表结构而不是集合；`TabsTrigger` 横向、纵向、`line`、文档站 `flex-none` 都含 `text-center` / `justify-center` 且不含 `justify-start` |
| SDK | JS / Go 创建体带 `data_model` 与 `init_sql`，schema 路径正确 |

验收命令：`go test ./...`，`cd ui && yarn test`，`cd ui && yarn typecheck`，`cd packages/js-sdk && yarn test && yarn typecheck`。

## 7. 不做

- 创建后修改 `data_model`，或把集合表自动迁成关系表。
- 删表、删列、改列类型、重命名。需要时仍可用 SQL 工作台（受 `sqlguard` 约束）。
- 改 DuckLake 文件格式、descriptor 或对象键。
- 恢复数据库打开 / 关闭。
- 改云助手的工具、权限、模型系统提示或 `/agents` 路径。
- 把设置 / 文档站整条页签栏改到页面水平正中。
