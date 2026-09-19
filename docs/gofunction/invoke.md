---
title: 调用指南
order: 2
---

# 调用云函数

## 端点

```
POST /go/{projectID}/{云函数名}/{导出函数名}
Authorization: Bearer <API_KEY>
Content-Type: application/json
```

- 成功 **200**：响应体就是函数返回值的 JSON（**没有** `{data:...}` 外层包装）。
- 只允许 **POST**；用浏览器地址栏 GET 会得到 405 JSON（而不是页面）。
- 权限：`DatabaseRead`（读写分离见 API 文档 §3.13）。

## Body 与入参的映射

| 入参类型 T | 请求 body | 示例 |
|---|---|---|
| struct / map / slice | JSON 对象/数组；**空 body 视为 `{}`** | `{"name":"a"}` |
| `string` | JSON 字符串字面量 | `"abc"` |
| `int` / `float64` 等数值 | JSON 数字字量 | `42` |
| `*Request`（指针） | 与 struct 相同，按 JSON 对象解 | `{"name":"a"}` |

## 示例

源码（控制台保存为 `hello.go`）：

```go
package main

type Request struct {
	Name string `json:"name"`
}

type Response struct {
	Message string `json:"message"`
}

func Hello(req Request) Response {
	return Response{Message: "hello, " + req.Name}
}
```

调用：

```bash
curl -X POST "$BASE/go/$PROJECT_ID/hello/Hello" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"name":"a"}'
```

响应（200）：

```json
{"message":"hello, a"}
```

基本类型入参：

```go
func Upper(s string) string { return s + "!" }
```

```bash
curl -X POST "$BASE/go/$PID/basic/Upper" -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" -d '"abc"'
# → "abc!"
```

## 错误码

| HTTP | code | 场景 |
|---|---|---|
| 400 | `invalid_request` | body 无法解到入参 |
| 401 | `unauthenticated` / `invalid_api_key` | 未带或错 Key |
| 404 | `gofunction_not_found` | 云函数不存在或已删除 |
| 404 | `function_not_found` | 导出函数不存在（含小写未导出） |
| 405 | `method_not_allowed` | 用了 GET |
| 504 | `request_timeout` | 执行超 10 秒（如死循环） |
| 500 | `gofunction_runtime_error` | 解释执行失败（message 已截断脱敏） |

## 可观测

每次调用会记录项目级指标（控制台「日志管理」与监控聚合可见）：`gofunction_invokes`（次数）、`gofunction_invoke_duration_ms`（耗时）、`gofunction_invoke_errors`（错误数）；保存时的编译耗时记录为 `gofunction_compile_ms`。
