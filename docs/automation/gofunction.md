---
title: 云函数概览
order: 10
group: 云函数
---

# 云函数（Go Function）

云函数让你在控制台保存一段 Go 源码，并把其中**导出的大写函数**通过统一 HTTP+JSON 接口对外暴露，无需构建与部署。

## 产品模型

| 概念 | 标识 | 例子 | 规则 |
|---|---|---|---|
| **云函数**（文件） | `name` | `hello` | 逻辑文件 `{name}.go`；同项目唯一 |
| **导出函数** | `functionName` | `Hello` | Go 导出函数（首字母大写）；调用时指定 |

一个云函数文件可导出多个函数；控制台列表会展示全部导出名，点击即可复制调用路径。

## HTTP+JSON 约定

每个**导出**函数必须形如：

```go
func Name(req T) R
```

- 恰好 **1 个入参**、**1 个返回值**（禁止 `(R, error)`、禁止无参或多参）。
- `T` / `R` 必须能被 `encoding/json` 编解码：struct / map / slice / 基本类型；struct 字段需导出，建议写 `json` tag。
- 包名必须是 `package main`。
- 未导出的 helper、`init` 可以存在，但不出现在列表、不可调用。
- 保存时服务端会**编译校验**全部导出函数；错误信息会指明具体函数与原因。

## 快速开始

1. 控制台侧栏打开 **云函数** → **新建云函数**。
2. 填写名称（如 `hello`），编辑器已预填模板：

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

3. 保存后列表出现 `hello.go` 与 `[Hello]` 徽标。
4. 调用（详见 [调用指南](/docs/automation/gofunction-invoke)）：

```bash
curl -X POST "$BASE/go/$PROJECT_ID/hello/Hello" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"name":"a"}'
# → {"message":"hello, a"}
```

## 限制与配额

| 项 | 值 |
|---|---|
| 单文件源码 | 256 KiB |
| 每项目数量 | 100 个 |
| 单次执行超时 | 10 秒（超时 504） |
| 可 import 的包 | 仅平台已注册标准库（fmt / strings / encoding/json / net/http 等） |
| 多文件 / go.mod / 第三方模块 | 不支持 |

## 安全说明

运行沙箱是**解释器**（非操作系统级沙箱）：脚本只能调用已注册的标准库符号；`net/http` 可发起出网请求。因此云函数源码的修改权等价于「持项目 API Key 的作者权限」，请妥善保管 Key。admin 系统项目不提供云函数。

## 使用场景

云函数不能直接读写项目的数据库、KV 或对象存储（没有内置 SDK）。需要访问这些资源时，在函数里用 `net/http` 回调 SimpleBase 自己的 HTTP API，并把 Key 作为入参传入，或让调用方在外部完成读写、函数只做计算。下面的例子都只用已注册的标准库。

### 场景一：统一的金额计算

背景：折扣和税费规则需要在多个服务里保持一致。把规则放进一个函数，调用方只传订单行。

```go
package main

type Line struct {
	Price float64 `json:"price"`
	Qty   int     `json:"qty"`
}

type Request struct {
	Lines       []Line  `json:"lines"`
	DiscountPct float64 `json:"discount_pct"`
	TaxPct      float64 `json:"tax_pct"`
}

type Response struct {
	Subtotal float64 `json:"subtotal"`
	Total    float64 `json:"total"`
}

func Calc(req Request) Response {
	sub := 0.0
	for _, l := range req.Lines {
		sub += l.Price * float64(l.Qty)
	}
	after := sub * (1 - req.DiscountPct/100)
	return Response{Subtotal: sub, Total: after * (1 + req.TaxPct/100)}
}
```

```bash
curl -sS -X POST "$BASE/go/$PROJECT/order/Calc" \
  -H "Authorization: Bearer $SB_KEY" -H 'Content-Type: application/json' \
  -d '{"lines":[{"price":199,"qty":2},{"price":89.5,"qty":1}],"discount_pct":10,"tax_pct":6}'
```

控制台保存时会编译校验；只有以大写字母开头、签名为「一个入参、一个返回值」的函数才会出现在调用列表里。

### 场景二：接收 Webhook 并做校验

背景：第三方回调只带一段 JSON，需要先校验字段再决定处理方式。函数只做校验与归一化，写库由调用方完成：

```go
package main

import "strings"

type Event struct {
	Type   string `json:"type"`
	Amount int    `json:"amount"`
}

type Result struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
	Kind   string `json:"kind,omitempty"`
}

func Handle(e Event) Result {
	if strings.TrimSpace(e.Type) == "" {
		return Result{OK: false, Reason: "type required"}
	}
	if e.Amount <= 0 {
		return Result{OK: false, Reason: "amount must be positive"}
	}
	return Result{OK: true, Kind: strings.ToLower(e.Type)}
}
```

函数返回错误信息而不是 panic。需要返回 HTTP 错误码的场景，应由调用方根据 `ok` 字段决定。

### 场景三：回调 SimpleBase 自身接口

背景：函数需要读取 KV 中的配置。`net/http` 的 `http.Get` / `http.NewRequest` 已注册，可以带上 Key 请求本服务。读取响应体用 `io.ReadAll`：

```go
package main

import (
	"io"
	"net/http"
)

type Input struct {
	URL string `json:"url"`
	Key string `json:"key"`
}

func Fetch(in Input) string {
	req, err := http.NewRequest(http.MethodGet, in.URL, nil)
	if err != nil {
		return err.Error()
	}
	req.Header.Set("Authorization", "Bearer "+in.Key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err.Error()
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err.Error()
	}
	return string(b)
}
```

注意：这里的 Key 来自调用方入参，不要写死在函数源码里；函数源码会保存在系统库中，修改权等价于项目 Key 的作者权限。单次执行有 10 秒上限，外部请求要考虑超时。

### 场景四：函数版本与回滚

每次保存会生成新版本，控制台「版本」里可以查看历史并把某个旧版本设为生效版，用来在发布出问题时快速回滚。管理接口为 `/v1/projects/:projectID/gofunctions/:name/versions`，其中 `.../versions/:ver/activate` 切换生效版、`.../versions/:ver/test` 以请求体 `{"function_name":"Calc","body":{...}}` 在线试跑。调用方始终调用同名函数，不需要改地址。

### 场景五：用 JS SDK 与 Go 代码调用

云函数就是普通 HTTP 接口，任何语言都可以直接调用。调用路径以 `/go/` 开头，不在 `/v1/projects/...` 下：

```ts
const res = await fetch(`${process.env.SIMPLEBASE_URL}/go/${process.env.SIMPLEBASE_PROJECT_ID}/order/Calc`, {
  method: 'POST',
  headers: {
    Authorization: `Bearer ${process.env.SIMPLEBASE_API_KEY}`,
    'Content-Type': 'application/json',
  },
  body: JSON.stringify({ lines: [{ price: 10, qty: 3 }], discount_pct: 0, tax_pct: 0 }),
})
console.log(await res.json())
```

## 常见问题

| 现象 | 可能原因 | 处理 |
|---|---|---|
| 保存时报编译错误 | 函数签名不是「一个入参、一个返回值」，或使用了未注册的标准库符号 | 对照上文约定，并查看[已注册包清单](https://github.com/linkxzhou/SimpleBase/blob/main/gofunction/README.md) |
| 调用返回 404 `function_not_found` | 函数名小写（未导出）或拼写错误 | 导出函数名必须首字母大写，与调用路径完全一致 |
| 调用返回 504 | 执行超过 10 秒 | 减少循环与外部请求；不要在函数里做长时间阻塞 |
| 用浏览器地址栏打开得到 405 | 只允许 `POST` | 使用 curl 或 `fetch` 的 `POST` |
