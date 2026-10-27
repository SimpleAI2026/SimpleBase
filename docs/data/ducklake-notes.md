---
title: 数据库引擎与约束
order: 2
group: 数据管理
---

# 数据库引擎与约束

每个项目的用户数据保存在一个 DuckDB 或 SQLite 单文件中，SQL、集合文档和[项目级 KV](/docs/data/kv) 共用这个文件。引擎在创建项目时通过 `engine` 选择（默认 `duckdb`），创建后不能更改。系统库仍使用 DuckLake，与用户库隔离；[DuckLake 上游参考](/docs/data/ducklake)只对系统库语法有参考意义，不代表用户库可用的能力。

## 引擎差异

DuckDB 与 SQLite 在类型、主键与唯一约束、自增和 upsert 语法上不同。需要同时支持两种引擎的应用，应只使用两者共有的标准 SQL，并在目标引擎上验证约束行为。

- 幂等写入：DuckDB 用 `MERGE INTO ... ON <业务键>`，SQLite 用 `INSERT ... ON CONFLICT`。
- 业务主键建议使用 UUID，或借助项目 [Key-Value `SET NX`](/docs/data/kv) 控制重复请求。
- `last_insert_rowid` 不通过 API 暴露，需要新生成的 ID 时使用 `RETURNING` 或应用侧生成。

## 一项目一数据库

每个项目只有一个数据库。已存在时再次创建会返回 409，删除该数据库只清理它自己的数据，不影响项目文件和其他资源。物理文件位置由服务端维护，不会出现在任何 API 响应中。

## SQL 安全边界

通过 HTTP `/query` 的 SQL 必须是只读查询；`/execute`、`/batch` 要求写权限。应用层 `sqlguard` 限定语句意图，禁止访问系统库、敏感路径和以 `__sb_` 为前缀的内部表（KV 等内部数据所在）。拒绝的 SQL 不会因为在浏览器 SQL 工作台粘贴而绕过服务端校验。

## 单写与远端同步

一个项目数据库同时只能有一个写实例，由项目级写租约保证；不能通过多开副本绕过限制。写入先在本地提交，启用 S3 时再以一致性快照同步到对象存储，同步有延迟，其他读实例看到的数据可能滞后。不要把远端对象立即可见当作写入已持久确认的唯一依据；运维细节见[部署指南](/docs/ops/deployment)。

## 适用场景

复杂查询使用[SQL 与集合](/docs/data/sql)；简单键值状态用[项目级 KV](/docs/data/kv)。SimpleBase 的权限、限制和 API 行为以本页和服务端错误为准。
