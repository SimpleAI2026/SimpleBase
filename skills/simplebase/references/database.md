# database
<!-- allow: database,sql,schema,collection,document -->
<!-- readonly: database list,database get,sql query,schema show,schema rows,collection list,document list -->

命令：`database list|get|create|delete`、`sql query|exec|batch`、`schema show|rows|create-table|add-column`、`collection list|create`、`document list|insert|update|delete`。

库有两种形态（`data_model`）：`sql`（SQL 与表结构）和 `collection`（集合与文档，建库默认）。形态不匹配返回 `data_model_mismatch`：SQL 库用 `sql`/`schema`，集合库用 `collection`/`document`。`--database` 传库 `id`，先查：

```
simplebase database list --limit 20
simplebase database get --id <库ID>
```

## 查询与写入（sql 库）

```
simplebase sql query --database <库ID> --statement "SELECT * FROM orders WHERE id = ?" --param "[1]" --max-rows 100
simplebase sql exec --database <库ID> --statement "INSERT INTO orders VALUES (1)"
simplebase sql batch --database <库ID> --file - --transactional   ← stdin 放 [{"sql":"...","args":[]}]
```

- `--param` 是 JSON 数组；长语句用 `--statement -` 从 stdin 读。
- `sql query` 只允许 SELECT/WITH/EXPLAIN/DESCRIBE/SHOW 等；写语句返回 `write_in_read_only`。
- `sql exec` 一次一条；多条用 `batch`（单次条数受 `max_batch_statements` 限制）。
- 多语句 `multiple_statements`、`ATTACH/COPY/SET/PRAGMA/CALL` 等管理语句 `sql_not_allowed`，直接改写语句。
- `sql exec` 中 `DROP/TRUNCATE/DELETE` 或无法判定的语句需要界面确认；`sql batch` 的内容是 JSON，宿主无法判定，一律需要确认。

## 表结构（sql 库）

```
simplebase schema show --database <库ID>
simplebase schema rows --database <库ID> --table orders --limit 20
simplebase schema create-table --database <库ID> --name orders --columns '[{"name":"id","type":"BIGINT","nullable":false},{"name":"note","type":"VARCHAR"}]'
simplebase schema add-column --database <库ID> --table orders --column '{"name":"qty","type":"INTEGER"}'
```

类型限 BOOLEAN/TINYINT/SMALLINT/INTEGER/BIGINT/FLOAT/DOUBLE/DECIMAL/VARCHAR/DATE/TIME/TIMESTAMP/BLOB/JSON/UUID；标识符须字母开头、仅字母数字下划线；一表最多 32 列。

## 集合与文档（collection 库）

```
simplebase collection list --database <库ID>
simplebase collection create --database <库ID> --name users
simplebase document list --database <库ID> --collection users
simplebase document insert --database <库ID> --collection users --json '{"name":"a"}'
simplebase document update --database <库ID> --collection users --id <文档ID> --json '{"name":"b"}'
simplebase document delete --database <库ID> --collection users --id <文档ID>   ← 需要确认
```

## 建库与删库

```
simplebase database create --name shop --data-model sql --init-sql -   ← 初始化 SQL 放 stdin，仅 sql 库可用
simplebase database delete --id <库ID>   ← 需要确认
```

`database delete`、`document delete`、`sql batch` 以及破坏性 `sql exec` 需要界面确认；headless 返回 `confirmation_unavailable`，不要重试。

## 常见错误码

`database_not_found`（核对库 ID）、`database_already_exists`、`invalid_database_name`、`system_database_protected`（系统库只读，不可写/删）、`system_column_redacted`、`row_limit_exceeded`（加 `LIMIT` 或调小 `--max-rows`）、`writer_unavailable`（实例只读）、`lease_acquiring`/`lease_held`（稍后重试一次）。
