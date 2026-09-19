---
title: 云函数概览
order: 1
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
4. 调用（详见 [调用指南](./invoke.md)）：

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
