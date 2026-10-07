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

文档采用 JSON 对象，插入未提供 `id` 时由服务端生成。文档列表最多返回 1000 条，按 `created_at` 降序；此接口不提供分页。集合读取需要 `database:read`，创建和文档写入需要 `database:write`；系统库不允许写操作。
