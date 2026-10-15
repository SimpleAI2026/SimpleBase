# 文档集合：建库、建集合、增删改查

默认 `data_model` 是 `collection`。集合名只能是字母开头的标识符，文档正文用 `--json`。

## 运行

```bash
export CASE=04-documents
./examples/capabilities/04-documents/run.sh
```

## 命令

```bash
simplebase database create --name "doc-$RUN_ID"
simplebase collection create --database "$DB" --name notes
simplebase document insert --database "$DB" --collection notes --json '{"title":"hello","body":"world"}'
simplebase document list --database "$DB" --collection notes
simplebase document update --database "$DB" --collection notes --id "$DOC" --json '{"title":"edited","body":"world"}'
simplebase document delete --database "$DB" --collection notes --id "$DOC"
simplebase database delete --id "$DB"
```

`document delete` 在云助手里需要确认；人类 CLI 直接删除。
