---
title: 数据库概览
order: 1
---

# 数据库

SimpleBase 用户数据面以 **DuckLake** 为主；开始操作请先看 [SQL 与集合](/docs/database/sql) 和 [DuckLake 使用须知](/docs/database/ducklake-notes)，上游原始资料另见 [DuckLake 参考](/docs/database/ducklake)。

在控制台 **数据库管理** 页可以：

- 创建数据库，并在创建时选择数据形态。创建成功即可使用，不需要打开或关闭
- **集合文档**（默认，存量库也是这种）：使用 SQL 工作台，并管理集合与文档
- **SQL 数据**：创建时可以提交初始化 SQL（`CREATE` / `ALTER` / `INSERT`），之后在页面上建表、加列
- 项目级 Key-Value 是独立资源，不挂在某个数据库下

## 约束与唯一性

DuckLake 不支持 `PRIMARY KEY`、`UNIQUE`、索引和序列；声明主键或唯一约束也不能保证数据唯一。业务主键建议使用 UUID，重复写入需在应用层通过 `MERGE INTO ... ON <业务键>`、KV `SET NX` 和事务内先查后写处理；后者依赖单写实例，不等价于数据库唯一约束。

平台限制与运维注意见 [DuckLake 使用须知](/docs/database/ducklake-notes)；详尽通用语法见 [DuckLake 上游参考](/docs/database/ducklake)。
