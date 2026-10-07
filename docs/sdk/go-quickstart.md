---
title: Go · 快速开始
order: 11
group: Go
---

# Go SDK 快速开始

本地先运行 `./build.sh dev`，在控制台创建一个普通用户数据库。开发模式提供种子 API Key `sb_live_dev_key_12345`；不要在生产环境使用或硬编码。设置 `SIMPLEBASE_API_KEY`、`SIMPLEBASE_PROJECT_ID` 和 `SIMPLEBASE_DATABASE_ID` 环境变量（示例项目 ID：`dev-shop`）。

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
)

func main() {
    client, err := gosdk.NewClient(gosdk.Options{
        URL: "http://127.0.0.1:8080",
        APIKey: os.Getenv("SIMPLEBASE_API_KEY"),
        ProjectID: os.Getenv("SIMPLEBASE_PROJECT_ID"),
        DatabaseID: os.Getenv("SIMPLEBASE_DATABASE_ID"),
    })
    if err != nil { log.Fatal(err) }
    result, err := client.Query(context.Background(), "SELECT ? AS answer", []any{42}, 0)
    if err != nil { log.Fatal(err) }
    fmt.Println(result.Columns, result.Rows)
}
```

`Rows` 中每行按 `Columns` 顺序排列；泛型 JSON 数值解码为 `float64`。若不想设置默认库，可用 `client.Database(databaseID).Query(...)`，而 `ListDatabases`、对象存储操作无需数据库 ID。每个请求传入 `context.Context`，可使用 `context.WithTimeout` 约束等待时间。

继续阅读 [数据库与 SQL](/docs/sdk/go-database-sql)。
