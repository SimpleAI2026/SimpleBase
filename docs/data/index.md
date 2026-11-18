---
title: 数据概览
order: 0
---

# 数据

SimpleBase 用户数据面以**项目级单文件数据库**为主：每个项目一个 DuckDB 或 SQLite 文件。控制台「数据」分组下有三类资源：**数据管理**、**Key-Value** 和**对象存储**。三者互相独立——Key-Value 与对象存储按项目隔离，不挂在某个数据库下。

| 子主题 | 适合存什么 | 文档 |
|---|---|---|
| 数据管理 | 结构化数据、集合文档 | [SQL 与集合](/docs/data/sql)、[数据库引擎与约束](/docs/data/ducklake-notes) |
| Key-Value | 缓存、计数、排行榜、临时状态 | [Key-Value](/docs/data/kv) |
| 对象存储 | 文件、图片、导出报表 | [对象存储](/docs/data/storage)、[对象存储 HTTP API](/docs/data/storage-api) |
| Realtime | 向客户端推送文档 API 写事件 | [文档 Realtime](/docs/data/realtime) |

## 怎么选

- 需要 `JOIN`、聚合、条件查询：用**数据库**。集合文档形态适合半结构化数据，SQL 形态适合明确表结构。
- 只需要按下发 key 存取：用 **Key-Value**，命令语义与 Redis 对齐，不需要建库建表。
- 存的是二进制或大文本（图片、CSV、日志）：用**对象存储**，它们不会作为 SQL 表出现。
- 临时状态加过期时间：Key-Value 支持 `EX` / `PX` 与 `ttl_ms`；对象存储没有 TTL。

## 控制台入口

「数据管理」页可以创建数据库并在创建时选择数据形态；创建成功即可使用，不需要打开或关闭。

- **集合文档**（默认，存量库也是这种）：使用 SQL 工作台，并管理集合与文档。
- **SQL 数据**：创建时可以提交初始化 SQL（`CREATE` / `ALTER` / `INSERT`），之后在页面上建表、加列。
- **数据浏览**：`/console/data` 是数据看板。选定集合后按字段类型自动生成默认看板（总记录、时间趋势、数值分布、分类占比），支持 9 种图表类型（自动、折线图、柱状图、条形图、饼图、表格、对比条形图、堆积条形图、散点图）、时间范围、全局筛选和点击图元联动下钻；下方是只读明细表。看板配置按「项目 + 库 + 集合」保存在浏览器本地。顶部输入框可以描述图表需求，由内置「数据」助手生成图表配置，预览确认后再加入看板，助手全程只读。Schema 设计器、新建视图、导入、导出收在「数据操作」菜单里；逻辑视图只读；编辑文档请到「数据管理」。
- **聚合接口**：看板的数据来自 `GET /v1/projects/{项目}/databases/{库}/data/collections/{集合}/aggregate`，需要 `database:read`。`metrics` 取 `count`、`sum:字段`、`avg:字段`、`min:字段`、`max:字段`、`distinct:字段`；`group_by` 取字段、`字段:day`（时间桶，另有 hour / week / month / year）或 `字段:bin=步长`；可再加 `series`、`filter`、`sort`、`limit`（最多 500）。`mode=points` 返回散点用的原始点（`x`、`y`，可选 `size`、`series`，最多 2000 个；是「按可用列排序的前 N 个点」，不保证最新、也不代表随机样本）。占比图可加 `with_total=1`：分组且只有一个 `count` / `sum` 度量时额外返回同口径总量 `total`，用于算真实分母。字段必须已登记 Schema，`created_at` / `updated_at` 恒可用。

## 约束与唯一性

约束行为取决于项目引擎（DuckDB 与 SQLite 对主键、唯一约束、自增和类型的处理不同），应用不应依赖未经验证的约束。业务主键建议使用 UUID，重复写入可通过 `MERGE INTO`（DuckDB）、`INSERT ... ON CONFLICT`（SQLite）、KV `SET NX` 和事务内先查后写处理；后者依赖单写实例。

平台限制与运维注意见[数据库引擎与约束](/docs/data/ducklake-notes)；系统库使用的 DuckLake 语法见 [DuckLake 上游参考](/docs/data/ducklake)。
