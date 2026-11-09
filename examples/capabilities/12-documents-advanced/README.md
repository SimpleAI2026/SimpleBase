# 文档进阶：patch / batch / export / import / schema / filter / truncate

在集合库上覆盖文档 API 的进阶能力：

1. `document patch`：merge 语义——只覆盖提交的字段，未提交字段保留；
2. `document batch`：一次写入多篇（1–100 篇）；
3. `collection schema-put`：注册字段后，`document list --filter/--fields/--count` 可用，
   缺 `required` 字段的写入被 400 拒绝（`error.code=required_field`）；
4. `document export`（NDJSON）→ `document import` 到新集合，行数一致；
5. `collection truncate` 清空集合。

## 前置

- 服务端已启动，`SIMPLEBASE_BIN` / `SIMPLEBASE_URL` / `SIMPLEBASE_TOKEN` / `SIMPLEBASE_PROJECT_ID` 已导出。
- 目标项目可以还没有用户数据库，用例会自己建；已有则直接复用（每个项目只允许一个用户数据库）。
- 用例自己生成带随机后缀的集合名，重复运行不会与旧集合冲突。

## 运行

```bash
export CASE=12-documents-advanced
./examples/capabilities/12-documents-advanced/run.sh
```

## 命令与接口

```bash
simplebase collection create --database "$DB" --name "notes_<suffix>"
simplebase document insert --database "$DB" --collection "notes_<suffix>" --json '{"title":"hello","body":"world","n":1}'
simplebase document patch  --database "$DB" --collection "notes_<suffix>" --id "$DOC" --json '{"body":"patched"}'
simplebase document batch  --database "$DB" --collection "notes_<suffix>" --json '[{"title":"b1","n":2},{"title":"b2","n":3}]'
simplebase collection schema-put --database "$DB" --collection "notes_<suffix>" \
  --json '{"fields":[{"name":"title","type":"text","required":true},{"name":"body","type":"text"},{"name":"n","type":"number"}]}'
simplebase document list --database "$DB" --collection "notes_<suffix>" --filter 'n >= 2' --fields title --count 1
simplebase document export --db "$DB" --collection "notes_<suffix>" --format ndjson --file /tmp/export.ndjson
simplebase document import --db "$DB" --collection "imported_<suffix>" --format ndjson --file /tmp/export.ndjson
simplebase collection truncate --database "$DB" --collection "imported_<suffix>"
```

对应 HTTP：`PATCH /documents/:id`、`POST /documents/batch`、`PUT /data/collections/:c/meta`、
`GET /export`、`POST /import`、`DELETE /data/collections/:c`。

## 断言点

- patch 后 `title` 仍为 `hello`、`body` 变为 `patched`、`n` 仍为 1（merge 不是整体替换）。
- batch 返回 2 篇，标题集合为 `{b1,b2}`；随后列表共 3 篇。
- schema-put 后有 3 个字段；`--filter 'n >= 2' --fields title --count 1` 返回 2 行、
  `total=2`，且行内只有 `title`（外加 id/时间戳），没有 `body`。
- export 成功且 NDJSON 恰好 3 行；import 返回 `scanned=3, imported=3`，目标集合列出 3 篇。
- 严格 schema 下插入缺 `title` 的文档失败，错误码 `required_field`；补齐后插入成功。
- truncate 后目标集合列表为 0 行。

## 已知边界

- `document list` 的 `filter` 只能过滤**已注册 schema 的字段**；未注册字段会返回校验错误。
  本用例先 `schema-put` 再过滤。
- `--fields` 只裁剪 `data` 层，`id/created_at/updated_at` 始终保留。
- import 会丢弃输入里的 `id`（服务端重新生成 UUID），因此 export→import 不保留原文档 id，
  只保证正文与行数。
- 集合与表建在项目单文件上；用例结束只删除自己新建的数据库，复用的数据库里会留下带随机后缀的集合。
