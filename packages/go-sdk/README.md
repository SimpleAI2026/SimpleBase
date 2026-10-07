# SimpleBase Go SDK

标准库实现的项目级 HTTP 客户端，位于仓库根模块的 `packages/go-sdk`；不包含独立 `go.mod`，目前不作为独立模块发布。使用 Go 1.25+，从仓库内导入：

```go
import gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
```

```go
client, err := gosdk.NewClient(gosdk.Options{
    URL: "http://127.0.0.1:8080", APIKey: os.Getenv("SIMPLEBASE_API_KEY"),
    ProjectID: os.Getenv("SIMPLEBASE_PROJECT_ID"),
})
if err != nil { log.Fatal(err) }
ctx := context.Background()
list, err := client.ListDatabases(ctx, 50, "")
if err != nil { log.Fatal(err) }
if len(list.Databases) == 0 { log.Fatal("create a user database first") }
result, err := client.Database(list.Databases[0].ID).Query(ctx, "SELECT ? AS answer", []any{42}, 0)
if err != nil { log.Fatal(err) }
fmt.Println(result.Columns, result.Rows)
```

示例依次需要导入标准库 `context`、`fmt`、`log`、`os`。提供数据库列表/创建/获取/删除，数据库级 SQL 与文档集合 CRUD，以及项目级对象存储的列表、上传、删除与预签名。所有方法接受 `context.Context`；可通过 `Options.HTTPClient` 注入自定义 HTTP 客户端。上传接收 `io.Reader` 并流式构造 multipart 请求；调用方负责关闭自己提供的文件或其他资源。

HTTP 非 2xx 错误可通过 `errors.As(err, &apiErr)` 提取 `*gosdk.APIError`（`Status`、`Code`、`Message`、`RequestID`）；事务批处理即使 HTTP 200 仍可能在 `BatchResult.Error` 返回回滚信息，非事务批处理逐项检查 `ErrorCode`。不要把长期 API Key 放在客户端代码或日志中。公共用法见 `docs/sdk/go-*.md`。

云沙盒 e2e 示例：

```go
result, err := client.RunSandbox(ctx, gosdk.SandboxRunInput{
    Image: "python:3.12-slim",
    Files: []gosdk.SandboxRunFile{{Path: "/workspace/test.py", Content: "print('ok')\n"}},
    SandboxExecInput: gosdk.SandboxExecInput{Command: "python test.py"},
})
if err != nil { log.Fatal(err) }
if result.TimedOut || result.ExitCode != 0 { log.Fatal(result.Stderr) }
```

`run` 同步执行，默认自动清理临时 VM。完整接口见 [`docs/sandbox/e2e.md`](../../docs/sandbox/e2e.md)。

验证：`go test ./packages/go-sdk`。
