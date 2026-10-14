---
title: 数据概览
order: 0
---

# 数据

SimpleBase 用户数据面以 **DuckLake** 为主。控制台「数据」分组下有三类资源：**数据库管理**、**Key-Value** 和**对象存储**。三者互相独立——Key-Value 与对象存储按项目隔离，不挂在某个数据库下。

| 子主题 | 适合存什么 | 文档 |
|---|---|---|
| 数据库管理 | 结构化数据、集合文档 | [SQL 与集合](/docs/data/sql)、[DuckLake 使用须知](/docs/data/ducklake-notes) |
| Key-Value | 缓存、计数、排行榜、临时状态 | [Key-Value](/docs/data/kv) |
| 对象存储 | 文件、图片、导出报表 | [对象存储](/docs/data/storage)、[对象存储 HTTP API](/docs/data/storage-api) |

## 怎么选

- 需要 `JOIN`、聚合、条件查询：用**数据库**。集合文档形态适合半结构化数据，SQL 形态适合明确表结构。
- 只需要按下发 key 存取：用 **Key-Value**，命令语义与 Redis 对齐，不需要建库建表。
- 存的是二进制或大文本（图片、CSV、日志）：用**对象存储**，它们不会作为 SQL 表出现。
- 临时状态加过期时间：Key-Value 支持 `EX` / `PX` 与 `ttl_ms`；对象存储没有 TTL。

## 控制台入口

「数据库管理」页可以创建数据库并在创建时选择数据形态；创建成功即可使用，不需要打开或关闭。

- **集合文档**（默认，存量库也是这种）：使用 SQL 工作台，并管理集合与文档。
- **SQL 数据**：创建时可以提交初始化 SQL（`CREATE` / `ALTER` / `INSERT`），之后在页面上建表、加列。

## 约束与唯一性

DuckLake 不支持 `PRIMARY KEY`、`UNIQUE`、索引和序列；声明主键或唯一约束也不能保证数据唯一。业务主键建议使用 UUID，重复写入需在应用层通过 `MERGE INTO ... ON <业务键>`、KV `SET NX` 和事务内先查后写处理；后者依赖单写实例，不等价于数据库唯一约束。

平台限制与运维注意见 [DuckLake 使用须知](/docs/data/ducklake-notes)；详尽通用语法见 [DuckLake 上游参考](/docs/data/ducklake)。
