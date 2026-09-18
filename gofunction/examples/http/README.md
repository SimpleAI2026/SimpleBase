# http

宿主用 `net/http/httptest` 起本地服务，把 URL 传给脚本；脚本调用已注册的 `http.Get` + `io.ReadAll`。不依赖外网。

仓库里访问 qq.com 的测试（`TestHttpRequest` / `TestHttpsRequest`）在 `go test -short` 下会跳过。

```bash
go run ./gofunction/examples/http
```

期望输出：`script http.Get => "pong"`
