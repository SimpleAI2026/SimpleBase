---
title: Go · 数据库与 SQL
order: 12
---

# 数据库与 SQL

以下示例假设已经按 [快速开始](/docs/sdk/go-quickstart) 创建 `client` 和 `ctx := context.Background()`。

```go
page, err := client.ListDatabases(ctx, 50, "")
if err != nil { return err }
if page.NextCursor != "" { /* 传入 NextCursor 读取下一页 */ }
created, err := client.CreateDatabase(ctx, "analytics")
if err != nil { return err }
db := client.Database(created.ID)
result, err := db.Query(ctx, "SELECT ? AS value", []any{42}, 100)
if err != nil { return err }
fmt.Println(result.Columns, result.Rows)
write, err := db.Execute(ctx, "INSERT INTO events (id) VALUES (?)", []any{"evt-1"})
if err != nil { return err }
fmt.Println(write.RowsAffected, write.Durability)
```

`GetDatabase(ctx, id)` 获取资源，`DeleteDatabase(ctx, id)` 返回 `{database_id,status}` 对应的 `DeleteDatabaseResult`（HTTP 202），不可当成数据库详情。创建和写入分别需要 `database:admin`、`database:write`；查询与列表需要 `database:read`。系统数据库只能 SELECT，不可写入或删除。SQL 请始终使用 `?` 参数占位符，不拼接不受信任的输入。

批处理由调用方明确选择事务模式：

```go
batch, err := db.Batch(ctx, []gosdk.SQLStatement{{SQL: "INSERT INTO events (id) VALUES (?)", Args: []any{"evt-2"}}}, true)
if err != nil { return err }
if batch.Error != nil { return fmt.Errorf("batch rolled back: %s", batch.Error.Code) }
for _, item := range batch.Results {
    if item.ErrorCode != "" { return fmt.Errorf("statement %d: %s", item.Index, item.ErrorCode) }
}
```

事务回滚可能以 HTTP 200 返回 `BatchResult.Error`；非事务批处理中检查每一项的 `ErrorCode`。`maxRows=0` 使用服务端行数上限，`last_insert_id` 不适用于 DuckLake，请使用 `RETURNING` 或应用生成 ID。查询行数、请求体与执行时间还有服务端限制。
