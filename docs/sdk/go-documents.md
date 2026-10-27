---
title: Go · 文档集合
order: 13
group: Go
---

# 文档集合

集合操作使用显式数据库 ID：`db := client.Database(databaseID)`。集合名称必须以英文字母开头，只能包含英文字母、数字和下划线，最长 63 字符。

```go
err := db.CreateCollection(ctx, "Notes")
if err != nil { return err }
collections, err := db.ListCollections(ctx)
if err != nil { return err }
fmt.Println(collections.Collections)

saved, err := db.InsertDocument(ctx, "Notes", gosdk.Document{"title": "第一条", "done": false})
if err != nil { return err }
id, ok := saved["id"].(string)
if !ok { return fmt.Errorf("missing document id") }
_, err = db.UpdateDocument(ctx, "Notes", id, gosdk.Document{"title": "已更新"})
if err != nil { return err }
list, err := db.ListDocuments(ctx, "Notes")
if err != nil { return err }
fmt.Println(list.Rows)
err = db.DeleteDocument(ctx, "Notes", id)
return err
```

文档采用 JSON 对象，插入未提供 `id` 时由服务端生成。`ListDocuments` 保留旧签名；`ListDocumentsWithOptions` 支持 `DocumentQuery{Limit,Cursor,Offset,Sort,Filter,Fields,Count}`，响应同时含旧 `Rows` 和结构化 `Items/NextCursor/Total`。`GetDocument`、`PatchDocument`、`BatchDocuments`、`TruncateCollection` 与 `Get/Put/DryRun/DeleteCollectionSchema` 对应单条读取、部分更新、单事务批量、清空和 Schema 管理。集合读取需要 `database:read`，创建和文档写入需要 `database:write`；系统库不允许写操作。

`ImportCollection(ctx, collection, format, dryRun, reader)` 流式上传 CSV/NDJSON；`ExportCollection(ctx, collection, query, writer)` 逐行写入调用方的 `io.Writer`，不受 JSON 响应 32MiB 上限约束。`DryRunView` 试跑逻辑视图。`List/Create/Update/DeleteFunctionTrigger` 管理[事件触发器](/docs/automation/function-triggers)，`InvokeFunctionTrigger` 调用 HTTP 触发器；`MigrationStatus/MigrationPlan` 只读检查 admin 项目的最终态系统表。
