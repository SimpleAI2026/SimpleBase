---
title: Go · 错误处理
order: 15
---

# 错误与取消

服务端非 2xx 状态返回 `*gosdk.APIError`：`Status`、`Code`、`Message`、`RequestID`。SDK 限制错误响应读取量，不保留原始错误正文；不要把凭据、SQL 参数或上传内容输出到日志。

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
_, err := client.Database(databaseID).Query(ctx, "SELECT 1", nil, 0)
if err != nil {
    var apiErr *gosdk.APIError
    if errors.As(err, &apiErr) {
        fmt.Printf("status=%d code=%s request=%s\n", apiErr.Status, apiErr.Code, apiErr.RequestID)
    } else if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
        fmt.Println("request canceled or timed out")
    } else {
        fmt.Println("request failed")
    }
}
```

常见错误包括 401 `invalid_api_key`、403 `forbidden`、404 `database_not_found`、409 `database_already_exists`、503 `writer_unavailable`。批处理的事务回滚可能返回 HTTP 200，必须检查 `BatchResult.Error`；非事务模式逐项检查 `BatchResult.Results[i].ErrorCode`。网络错误保留原始原因以供 `errors.Is` / `errors.As` 判断；本地参数校验错误不含 HTTP 状态。
