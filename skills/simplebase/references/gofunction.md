# 云函数（gofunction）

云函数由服务端进程内的 Go 解释器执行，不是真正的 `go build`。本地 IDE 能编译不代表服务端能跑，以 `function test` 的结果为准。

## 源码约定

- 单文件、`package main`。建议在第一行加 `//go:build ignore`，避免被所在仓库的 `go build ./...` 拾取。
- 每个**大写**顶层函数都是可调用的导出函数，必须恰好 1 个入参、1 个返回值，如 `func Hello(in HelloInput) HelloOutput`。入参和返回值可以是 struct（字段大写，带 `json` tag）、`map[string]interface{}` 或基础类型。小写函数不能被调用，可以作为辅助函数。
- 只能 import：`fmt strings math time regexp strconv errors bytes io io/ioutil sync/atomic net/http encoding/json encoding/base64 encoding/binary github.com/json-iterator/go`，而且只包含常用符号：`strings` 没有 `Builder`，`fmt` 没有 `Fprintf`，没有 `os`/`sort`/`sync`/`context`。引用未注册的符号会编译失败。
- 支持 struct、slice/map、闭包、`defer`、`panic/recover`、`go` 与 channel；不支持 cgo 和 `go.mod`。单次执行默认 10 秒超时。`print`/`println` 的输出不会返回，调试时返回结构化结果。
- 限制：源码 256KiB；每个项目最多 100 个函数，每个函数最多 50 个版本（重放按 sha 去重，源码不变不会新建版本）。
- 函数里不访问数据库连接或文件系统；需要持久化时，由调用方（BFF）通过 SDK 读写，或经 `net/http` 调用本项目 API。不要在源码里写 Key。

## 示例

```go
//go:build ignore

package main

import (
	"errors"
	"strings"
)

type QuoteInput struct {
	Items  []Line `json:"items"`
	Coupon string `json:"coupon"`
}

type Line struct {
	PriceMinor int64 `json:"price_minor"`
	Qty        int64 `json:"qty"`
}

type QuoteResult struct {
	TotalMinor int64 `json:"total_minor"`
}

func Quote(in QuoteInput) (QuoteResult, error) { // 错误：两个返回值不符合约定
	return QuoteResult{}, errors.New("x")
}
```

上面的 `Quote` 会报 `gofunction_signature_invalid`。改为只返回一个值，把错误放进结果字段：

```go
type QuoteResult struct {
	TotalMinor int64  `json:"total_minor"`
	Error      string `json:"error,omitempty"`
}

func Quote(in QuoteInput) QuoteResult {
	var r QuoteResult
	for _, l := range in.Items {
		if l.Qty < 0 {
			r.Error = "qty must be >= 0"
			return r
		}
		r.TotalMinor += l.PriceMinor * l.Qty
	}
	if strings.ToUpper(in.Coupon) == "DEMO10" {
		r.TotalMinor -= r.TotalMinor / 10
	}
	return r
}
```

## 验证

```bash
simplebase function invoke --name pricing --export Quote --input '{"items":[{"price_minor":100,"qty":2}]}'
simplebase function test --name pricing --version 3 --export Quote --input '{}'
```

`invoke` 只运行生效版本，`data` 就是返回值。`test` 可以运行任意版本，`data` 是信封 `{ok,status_code,duration_ms,version,function_name,data,error}`，返回值在 `data.data`。

## 常见错误码

- `gofunction_signature_invalid`：导出函数签名不符合约定。
- `gofunction_compile_error`：语法错误或引用了未注册的符号，按 message 修改。
- `gofunction_runtime_error` / `request_timeout`：运行时 panic 或超时。
- `function_not_found`：`--export` 不存在；`no_active_version`：先 activate。
- `version_limit_exceeded`：50 个版本已用完，需要在控制台清理（不要自动删除）。
- `too_many_go_runs`：并发已满，稍后重试一次。
