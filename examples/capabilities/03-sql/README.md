# SQL 表：建库、建表、加字段、增删改查

在 `data_model=sql` 的库上走 `schema` 与 `sql` 命令。库名带 `RUN_ID`，避免重复执行撞名。

DuckLake 不使用 `PRIMARY KEY` / `UNIQUE` / 索引。列类型用 `BIGINT`、`VARCHAR`、`INTEGER`。

## 运行

```bash
export CASE=03-sql
./examples/capabilities/03-sql/run.sh
```

## 命令

```bash
simplebase database create --name "sql-$RUN_ID" --data-model sql
simplebase schema create-table --database "$DB" --name items \
  --columns '[{"name":"id","type":"BIGINT","nullable":false},{"name":"title","type":"VARCHAR"}]'
simplebase schema add-column --database "$DB" --table items \
  --column '{"name":"qty","type":"INTEGER"}'
simplebase sql exec --database "$DB" --statement "INSERT INTO items (id, title, qty) VALUES (1, 'demo', 3)"
simplebase sql query --database "$DB" --statement "SELECT id, title, qty FROM items" --max-rows 10
simplebase sql exec --database "$DB" --statement "UPDATE items SET title = 'edited' WHERE id = 1"
simplebase sql exec --database "$DB" --statement "DELETE FROM items WHERE id = 1"
simplebase schema show --database "$DB"
simplebase schema rows --database "$DB" --table items --limit 10
simplebase database delete --id "$DB"
```

`database delete`、破坏性 `sql exec` 在云助手里需要界面确认；本脚本是人类 CLI，直接执行。
