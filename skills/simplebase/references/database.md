# 数据库、迁移与集合

## 两种库

- `sql` 库：表结构和 SQL。迁移只对 sql 库有效。
- `collection` 库：集合和文档（JSON）。创建库时的缺省类型是 collection，所以清单里有迁移的库必须是 sql（不写 `data_model` 时脚本会自动推断）。
- 引擎（duckdb / sqlite）是**项目级**设置，在建项目时决定，库上不能单独指定。同一个工作区的迁移只按一种方言写，本地和云端项目的引擎要一致；plan / apply 会读 `project list` 比对 lock 里记录的各环境引擎，不一致时给出警告。

库 ID 在每个实例上都不同，不要硬编码。脚本会把 ID 写进 lock 和 `.simplebase/env.<label>`；业务代码可以读 `SIMPLEBASE_DATABASE_ID_<NAME>`，或在启动时 `database list` 后按名称查找。

## 迁移写法

- 文件名决定执行顺序：`001_init.sql`、`002_orders.sql`……已应用的文件**不能改**（sha 不一致会报 `migration_modified`），只能新增文件。
- 一个文件在一个事务里执行（`sql batch --transactional`），失败时整个文件回滚，修好后重跑即可。
- 语句之间用 `;` 分隔。拆分时会跳过引号（`'...'`、`"..."`、`$$...$$`）与 `--` / `/* */` 注释里的分号，所以字面量里可以写 `;`；注释会被去掉。
- 每个文件最多 99 条语句（批量上限是 100 条，其中 1 条用于记账）。
- 用 `CREATE TABLE IF NOT EXISTS`、`ALTER TABLE ... ADD COLUMN`。不要写 `PRIMARY KEY`、`UNIQUE`、`CREATE INDEX`、`CREATE SEQUENCE`（DuckLake 不支持），需要唯一性时在写入逻辑里保证（如先查后写、用幂等键列）。
- 被 sqlguard 拒绝的语句：`ATTACH/DETACH/INSTALL/LOAD/COPY/EXPORT/IMPORT/SET/PRAGMA/CHECKPOINT/CALL/USE/VACUUM/PREPARE/EXECUTE`、`CREATE/DROP SECRET|MACRO`，以及多语句字符串。
- 不能引用保留前缀 `__sb_`、`__ducklake_metadata_`、`sqlite_`，也不要碰记账表 `_migrations`。
- 迁移里的 `DROP`/`DELETE`/`TRUNCATE` 会产生警告；发布到云端前要让用户确认。
- 列类型建议使用 BOOLEAN/INTEGER/BIGINT/DOUBLE/DECIMAL/VARCHAR/DATE/TIMESTAMP/JSON/UUID。时间统一用 ISO 字符串或 TIMESTAMP，不混用。

```sql
-- 002_orders.sql
CREATE TABLE IF NOT EXISTS orders (id VARCHAR, user_id VARCHAR, total_minor BIGINT, idem_key VARCHAR, created_at VARCHAR);
ALTER TABLE notes ADD COLUMN IF NOT EXISTS tags VARCHAR;
```

## 种子

- `databases[].seed`（SQL）和 `collections[].seed`（文档）只在 `--seed` 时执行，各执行一次（SQL 按文件记账，文档按 id 去重）。
- 只放小的、可重建的开发夹具；不要放真实用户数据，也不要对生产环境执行 `--seed`。

## 查询与排查

```bash
simplebase database list --limit 20
simplebase schema show --database <库ID>
simplebase sql query --database <库ID> --statement "SELECT name, applied_at FROM _migrations ORDER BY name"
simplebase sql query --database <库ID> --statement "SELECT * FROM orders WHERE id = ?" --param '["o1"]' --max-rows 50
simplebase collection list --database <库ID>
simplebase document list --database <库ID> --collection posts --limit 20
```

`sql query` 只允许 SELECT/WITH/EXPLAIN/DESCRIBE/SHOW；写语句用 `sql exec`（一次一条）或 `sql batch`。在云端手工执行写语句之前，先征得用户同意，并且优先把它写成新的迁移文件。

## 常见错误码

`data_model_mismatch`（库类型不匹配）、`database_already_exists`、`database_not_found`、`sql_not_allowed`、`multiple_statements`、`write_in_read_only`、`init_sql_failed`、`row_limit_exceeded`（加 LIMIT）、`writer_unavailable`（实例只读）、`lease_acquiring`/`lease_held`（稍后重试一次）、`system_database_protected`（系统库只读）。
