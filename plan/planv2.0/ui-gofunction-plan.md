# 云函数（Go Function）控制台 + HTTP 调用计划

> 状态：**implemented（2026-09-18 落地；r1 修订意见已吸收进实现）**
> 日期：2026-09-18
> 范围：`ui/`（侧栏 + 列表 + Monaco 编辑弹窗）· `internal/`（Echo CRUD + `/go` 调用）· `gofunction/`（签名校验 + JSON 入参/出参）
> 配套：[`ui-plan-v2.md`](./ui-plan-v2.md)、[`ui-principles.md`](./ui-principles.md)、[`proto-http.md`](./proto-http.md)、[`gofunction-simplify-plan.md`](./gofunction-simplify-plan.md)
> 约束：沿用 Vue 3 + shadcn-vue / Tailwind v4、Echo、系统 DuckLake；**Monaco 是用户点名的编辑器**（见 §8，属明确允许的依赖例外）

---

## 1. 目标

把已经落地的 `gofunction` 解释器接到控制台与 Echo：每个项目有一组 **`.go` 源文件**（云函数），每个文件可导出多个 **大写函数名**；保存后即可用统一 HTTP+JSON 对外调用。

| # | 需求 | 落地形态 |
|---|---|---|
| 1 | 侧边栏「云函数」 | 路由 `gofunctions`，点开右侧列表页 |
| 2 | 列表新增 / 删除 / 查看 | 与 Databases / S3 同构的 Card + Table |
| 3 | 新增或编辑弹出统一 modal | `SbModal` + Monaco，编辑 Go 源码 |
| 4 | 列表展示导出函数 Name | 保存时用 `gofunction.ParseFuncList(src, false)`，只展示包级导出（大写）函数 |
| 5 | `/go/{项目ID}/{云函数名}/{functionName}` | 加载 `{云函数名}.go`，执行 `{functionName}`；**POST + JSON**；**入参 1 个、返回值 1 个**；创建弹窗里写死提示 |

一句话：

```
控制台保存 hello.go  ──►  ParseFuncList 得到 [Hello]
对外 POST /go/{projectID}/hello/Hello  + JSON body
                           │
                           ▼
              gofunction.RunJSON(src, "Hello", body) → JSON
```

---

## 2. 现状（已核实，不编造）

| 层 | 现状 |
|---|---|
| `gofunction` | 公共 API 已有 `Run` / `BuildProgram` / `ParseFuncList`；`ParseFuncList(..., false)` **只返回包级导出函数**（跳过方法、`init`、未导出）。`packages` 已注册 `fmt` / `strings` / `encoding/json` / `net/http` 等。**主工程 `internal/` 尚未 import 本包。** |
| JSON 绑定缺口 | `Run` 的 `params` 经 `value.ValueOf` 装箱，**没有** JSON → 形参类型的绑定。用户 struct 在解释器里由未导出的 `typeChange` + `reflect.StructOf` 生成，且会拷贝源码 struct tag，因此 **JSON 绑定必须做在 `gofunction` 包内**（才能用 `typeChange`）。 |
| UI | 侧栏由 `router/index.ts` 的 `meta.title` + `NavMenu.iconMap` / `routeOrder` 驱动。无 Monaco、无云函数页。历史计划里的 FaaS 菜单被隐藏且后端不存在——**本计划替代那条假能力**。 |
| 存储 | 系统 DuckLake 迁移目前最高 **v27**（`sys_agent_schedule_runs`）。`objectstore.FileStore` 只有 List/Put/Delete/PresignGet，**没有 Get 读 body**，不适合作为 invoke 热路径。 |
| Echo | `mountV1Routes` 挂 `/v1/projects/:projectID/*`；`web.Register` 的 SPA fallback **只拦 GET `/*`**。`/go/...` 必须在 fallback **之前**注册。Vite 代理目前只有 `/v1`、`/health`。 |

---

## 3. 产品模型

两个名字不要混：

| 概念 | 标识 | 例子 | 规则 |
|---|---|---|---|
| **云函数**（文件） | `name` | `hello` | 逻辑文件 `{name}.go`；路径段、列表主键 |
| **导出函数** | `functionName` | `Hello` | Go 导出函数（`IsExported`）；invoke 最后一段 |

一个云函数文件可导出多个函数，列表用 Badge 列出全部 Name。调用时必须指定其中一个。

```
hello.go
  func Hello(req Request) Response   →  POST /go/{pid}/hello/Hello
  func Ping(req Request) Response    →  POST /go/{pid}/hello/Ping
```

### 3.1 HTTP 函数约定（创建时提示，保存时强制校验）

每个**导出**函数必须是：

```go
func Name(req T) R
```

- 恰好 **1 个入参**、**1 个返回值**（禁止 `(R, error)`、禁止无参、禁止多参）。
- `T` / `R` 必须能被 `encoding/json` 编解码：struct / map / slice / 基本类型；struct 字段需导出，建议写 `json` tag。
- 允许 `T` 为指针（`*Request`）；body 仍按 JSON 对象解。
- 包名必须是 `package main`。
- 未导出的 helper、`init` 可以存在，不出现在列表、不可通过 `/go` 调用（即使知道名字也不开放）。

创建弹窗默认模板（见 §7.3）。

### 3.2 明确不做（MVP）

- 不把源码混进用户 S3 列表（`FileStore` 无 Get；也避免和对象存储互相干扰）。
- 不给脚本注入 SQL / S3 / catalog 宿主 API（可后续 `importer.RegisterPackage`）。
- 不支持多文件、`go.mod`、第三方模块。
- 不支持 GET/query 调函数、不支持流式/SSE、不支持异步任务队列。
- 不公开无鉴权的 invoke（对外服务仍带项目 API Key）。
- 不改 JS SDK（`js-sdk-plan.md` 可列 Phase 2）。
- 不做分布式编译缓存；每次调用 `BuildProgram`（可在同进程做按 `updated_at` 的 `*Program` 缓存，见 §6.4，非必须）。
- admin 系统项目（`ReservedSystemProjectID`）**不提供**云函数（与系统库只读策略一致）：列表空态 + 隐藏新建。

---

## 4. 架构

```
┌─ UI (Vue) ──────────────────────────────────────────┐
│  NavMenu 「云函数」 → pages/GoFunctions.vue          │
│  新建/编辑/查看 → GoFunctionModal + Monaco           │
│  api.gofunctions.*  ← types.ts / http-api / mock     │
└──────────────┬──────────────────────────────────────┘
               │  Bearer  /v1/projects/:pid/gofunctions
               ▼
┌─ Echo ──────────────────────────────────────────────┐
│  CRUD  GoFunctionHandler  (auth + project context)  │
│  Invoke POST /go/:pid/:name/:functionName           │
│         同套 API Key + projectContext                │
└──────────────┬──────────────────────────────────────┘
               │
               ▼
┌─ systemdb.sys_gofunctions ─┐    ┌─ gofunction ─────────┐
│  name / source / exports   │───▶│  ParseFuncList        │
│  源码为权威，exports 冗余   │    │  ValidateHTTPFuncs    │
└────────────────────────────┘    │  RunJSON(src, fn, body)│
                                  └──────────────────────┘
```

依赖方向：`api` → `systemdb` + `gofunction`；`gofunction` **不得** import `internal/*`。

装配：`app.assembleDeps` 把 `System` 注入 handler；在 `gofunction_handler.go`（或 `internal/gofn` 若拆包）**blank-import** `gofunction/packages`，否则脚本 `import "fmt"` 会编不过。

---

## 5. 数据模型（系统 DuckLake）

`internal/systemdb/migrate.go` 追加只前进迁移 **v28**。DuckLake 不用 PRIMARY KEY；唯一性由应用层保证。软删与 agents 一致（`archived_at`）。

### v28 `sys_gofunctions`

| 列 | 类型 | 说明 |
|---|---|---|
| `id` | VARCHAR | UUID |
| `project_id` | VARCHAR | 项目隔离 |
| `name` | VARCHAR | 云函数名，即 `{name}.go` 的 basename；同项目唯一（未归档） |
| `source` | VARCHAR | 完整 Go 源码 |
| `exports_json` | VARCHAR | `["Hello","Ping"]`，保存时由 `ParseFuncList(src, false)` 写入 |
| `created_at` / `updated_at` | TIMESTAMP | — |
| `archived_at` | TIMESTAMP | 软删 |

`name` 规则与集合名相同：`^[A-Za-z][A-Za-z0-9_]{0,62}$`。API 与路径里**不带** `.go` 后缀；UI 展示时拼上。

源码上限：**256 KiB**（同时受 `limits.max_request_bytes` 约束，取更严者）。每项目未归档云函数上限：**100**。

### Store（`internal/systemdb/gofunctions.go`）

风格对齐 `agents.go`：全部 project 作用域 + `notifyWrite`。

```go
ListGoFunctions(ctx, projectID string) ([]GoFunction, error)
GetGoFunction(ctx, projectID, name string) (GoFunction, error) // ErrNoRows → 404
CreateGoFunction(ctx, row GoFunction) (GoFunction, error)
UpdateGoFunction(ctx, row GoFunction) (GoFunction, error)
ArchiveGoFunction(ctx, projectID, name string) error
CountGoFunctions(ctx, projectID string) (int, error)
```

模型字段与表一一对应；`Exports []string` 由 JSON 列编解码。

---

## 6. gofunction 扩展（给 HTTP 用的公共 API）

文件建议：`gofunction/jsonrun.go` + `jsonrun_test.go`。不改解释器主循环。

```go
type FuncInfo struct {
    Name       string // Hello
    ParamType  string // main.Request 或 map[string]interface{}
    ResultType string
}

// ValidateHTTPFuncs：ParseFile + BuildProgram，
// 对每个包级导出函数要求 Signature.Params==1 且 Results==1。
// 除签名外，还必须对每个导出函数的形参/返回值类型预演 typeChange：
// 用户 struct 含未导出字段或解释器不支持的类型时返回 error，
// 把问题提前到保存时（400），而不是 invoke 时才 panic。
// 编译失败或签名不合返回 error（带位置信息，可映射 400）。
func ValidateHTTPFuncs(source string) ([]FuncInfo, error)

// RunJSON：BuildProgram → 定位函数 → json.Unmarshal 到形参 reflect 值 → Program.Run → json.Marshal 返回值。
// body 为空视为 {}。函数不存在返回明确 error。
func RunJSON(ctx context.Context, seqid, source, funcName string, body []byte) ([]byte, error)
```

实现要点：

1. `BuildProgram(seqid, name, source)`，`fname` 用云函数名（生成 `{name}.go` 便于报错）。
2. `mainFn := program.mainPkg.Func(funcName)`；`nil` → `function not found`；**非导出（`!ast.IsExported` 首字母大写判断）→ 同样返回 `function not found`**，与 §3.1「知道名字也不开放」一致。
3. `sig := mainFn.Signature`；校验 arity。
4. `rt := typeChange(sig.Params().At(0).Type())`；`ptr := reflect.New(derefIfPtr(rt))`；`json.Unmarshal(body, ptr.Interface())`。
5. `program.RunWithContext(seqid, funcName, ptr.Elem().Interface())`（指针形参则传 `ptr.Interface()`）。
6. 返回值 `json.Marshal`；`nil` 返回 `null`。
7. **超时识别**：解释器超时以 `panic(ctx.Err())` 形式从 `runFrame` recover，
   最终包成 `fmt.Errorf("err: %v", fr.panic)` / `"recover: %v"`，`errors.Is` 对
   `context.DeadlineExceeded` 会失配。`RunJSON` 必须在返回前用
   `strings.Contains(err.Error(), context.DeadlineExceeded.Error())`（或定义
   `ErrTimeout` 哨兵包一层）识别，供 handler 映射 504。
8. **ctx 用途**：解释器内部 Context 由 `newCallContext()` 自建（基于
   `context.Background()` + 10s），不接收外部 ctx 注入。`RunJSON` 的 `ctx` 参数
   仅用于执行前后检查调用方取消（客户端断开 / 网关超时）：进入时 `ctx.Err() != nil`
   直接返回该错误；解释器返回后若 `ctx.Err() != nil` 且非 DeadlineExceeded，优先
   返回调用方取消。

`ParseFuncList` 继续给列表用（只解析、不编译）。保存路径：**先** `ValidateHTTPFuncs`（含编译 + typeChange 预演），通过后再写库，并把 `[]FuncInfo.Name` 存进 `exports_json`。

`ValidateHTTPFuncs` 失败时错误信息**必须列出每个不合规导出函数的名字与原因**（如「Hello: 期望 1 个入参，实际 2」「Ping: 参数类型含未导出字段」），否则用户无法定位；handler 原样透传该 message。

测试（`gofunction` 包内）：

- struct + `json` tag 往返
- `map[string]interface{}` 往返
- 基本类型入参（`func F(s string) string`）：body 为 JSON 字符串字面量 `"abc"` 时正确绑定
- 多参 / `(R, error)` / 无导出函数 → error
- 非法 Go / 未注册标准库符号 → compile error
- 未导出函数不能 `RunJSON`（返回 function not found，而非执行）
- struct 含未导出字段 → `ValidateHTTPFuncs` 报错（保存拦截）
- 解释器超时（死循环脚本）→ `RunJSON` 返回可识别为 DeadlineExceeded 的 error

---

## 7. HTTP 契约

落地时同步写入 `proto-http.md` §3.13 与 `proto.http`。前端 `types.ts` 与下表一一对应。

认证：与现网一致 `Authorization: Bearer <API_KEY>`。CRUD 与 invoke **都要** Key（对外服务 ≠ 匿名）。权限：读 `DatabaseRead`，写 `DatabaseWrite`；只读实例写接口 → `503 writer_unavailable`。

### 7.1 管理面（控制台）

前缀 `:p` = `/v1/projects/:projectID`。

| Method | Path | 权限 | 成功 | 说明 |
|---|---|---|---|---|
| GET | `:p/gofunctions` | DatabaseRead | 200 | `{ "functions":[ ... ] }` |
| POST | `:p/gofunctions` | DatabaseWrite | 201 | 创建；name 冲突 409 |
| GET | `:p/gofunctions/:name` | DatabaseRead | 200 | 详情含 `source` |
| PUT | `:p/gofunctions/:name` | DatabaseWrite | 200 | 更新源码（name 不可改） |
| DELETE | `:p/gofunctions/:name` | DatabaseWrite | 204 | 软删 |

`GoFunction` JSON（snake_case）：

```json
{
  "id": "…",
  "name": "hello",
  "file": "hello.go",
  "source": "package main\n…",
  "exports": ["Hello", "Ping"],
  "created_at": "2026-09-18T12:00:00Z",
  "updated_at": "2026-09-18T12:00:00Z"
}
```

列表可省略 `source`（减小 payload）；详情 / 创建 / 更新返回完整 `source`。`file` 由服务端派生 = `name + ".go"`，客户端不回传。

创建 body：`{ "name": "hello", "source": "..." }`。`name` 必填：为空或不符合规则 → 400 `invalid_gofunction_name`（包名恒为 `main`，不从源码猜名）。

更新 body：`{ "source": "..." }`。

保存时服务端：

1. 校验 `name`、源码非空、体积。
2. `ValidateHTTPFuncs`；失败 400 `gofunction_compile_error` 或 `gofunction_signature_invalid`（message 带编译器/约定原文——含每个不合规函数名与原因，前端原样展示）。
3. 至少 1 个导出函数，否则 400 `gofunction_signature_invalid`（「至少导出一个大写函数」）。

### 7.2 调用面（对外 HTTP+JSON）

```
POST /go/:projectID/:name/:functionName
Content-Type: application/json
Authorization: Bearer <API_KEY>

{ ... }          → 映射到函数唯一入参
```

成功 **200**：响应体就是返回值的 JSON（**无** `{data:...}` envelope，与「函数返回值即 HTTP 体」一致）。

| 情况 | HTTP | code |
|---|---|---|
| 云函数不存在 | 404 | `gofunction_not_found` |
| 导出函数不存在（或未导出） | 404 | `function_not_found` |
| JSON 无法解到入参 | 400 | `invalid_request` |
| 解释器 panic / 运行错误 | 500 | `gofunction_runtime_error`（message 截断脱敏，不含源码全文） |
| invoke 时 `BuildProgram` 失败（保存后环境变化） | 500 | `gofunction_compile_error` |
| 执行超时（默认 10s，见 §6 要点 7） | 504 | `request_timeout` |

只允许 POST。GET `/go/...` 若不注册会掉进 SPA HTML——**必须同步注册 GET 返回 405**：

```json
{"error":{"code":"method_not_allowed","message":"use POST with JSON body"}}
```

基本类型入参的 body 形态：入参为 `string` 时 body 是 JSON 字符串字面量（`"abc"`）；入参为 `int` 时是数字字面量；struct/map 为 JSON 对象。「空 body 视为 `{}`」仅对 struct/map/slice 入参有意义——文档与 Alert 文案需写明此差异。

路由挂载（`NewRouter`，在 `web.Register` 之前，与 `mountV1Routes` 同一 `deps.Auth != nil && deps.DatabaseHandler != nil` 守卫内，复用其 `authMW` / `require` 构造，避免依赖未装配时挂出半残路由）：

```go
goGrp := e.Group("/go/:projectID", authMW, projectContextMiddlewareEcho(deps))
goGrp.POST("/:name/:functionName", h.Invoke, require(auth.DatabaseRead))
goGrp.GET("/:name/:functionName", methodNotAllowed) // 405 JSON，防 SPA fallback 吞掉
```

`name` / `functionName` 做与 CRUD 相同的字符校验，防止路径奇怪字符。

Vite：`server.proxy` 增加 `'/go': { target: apiTarget, changeOrigin: true }`（`ui/AGENTS.md` / `ui-principles.md` 规定新前缀必须补代理）。

### 7.3 创建时的固定提示（产品文案）

弹窗顶部 Alert（不可关掉，只读）：

> **HTTP+JSON 约定**：每个导出函数必须是 `func Name(req T) R`（1 个入参、1 个返回值）。
> 调用：`POST /go/{当前项目ID}/{云函数名}/{Name}`，`Content-Type: application/json`。
> Body 映射到 `req`，响应体是 `R` 的 JSON。请使用大写函数名，否则不会出现在列表、也无法调用。
> 入参为基本类型时，body 用对应 JSON 字面量（如 `"abc"`）；入参为 struct 时用 JSON 对象。

新建时 Monaco 预填：

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

---

## 8. UI

原则对齐 `ui/AGENTS.md` / `ui-principles.md`：PageContainer 只有 subtitle；主区铺满；确认用 `ConfirmAction`；提示用 `vue-sonner`；空态 `SbEmptyState`；分页 `TablePager`。

### 8.1 信息架构

| 项 | 值 |
|---|---|
| 路由 | `path: 'gofunctions'`, `name: 'gofunctions'`, `meta.title: '云函数'` |
| 侧栏顺序 | dashboard → databases → s3 → **gofunctions** → agents → logs（Settings 已离开侧栏，见 [`ui-settings-merge-plan.md`](./ui-settings-merge-plan.md)） |
| 图标 | `@lucide/vue` 的 `CodeIcon`（若当前版本导出 `SquareFunctionIcon` 则优先） |
| 页面 | `ui/src/pages/GoFunctions.vue` |
| 弹窗 | `ui/src/components/modal/GoFunctionModal.vue` |
| 编辑器 | `ui/src/components/editor/GoMonacoEditor.vue` |

`NavMenu.iconMap` / `routeOrder` 必须同步，否则新路由进不了侧栏。

### 8.2 样式 ASCII：壳 + 侧栏选中

```
┌─ Sidebar (w-56 / icon 折叠) ─┬─ Inset ──────────────────────────────────────────────┐
│ [S] SimpleBase               │ ☰  云函数          使用文档  [商城后台 ▾]  ⚙  ↻     │
│                              ├──────────────────────────────────────────────────────┤
│  📊  监控大盘                 │  管理项目内的 Go 源文件；导出的大写函数可通过 HTTP+JSON 调用
│  🗄  数据库管理               │
│  ☁   S3 对象存储              │  ┌─ Card ─────────────────────────────────────────┐
│  </> 云函数        ← active  │  │ 云函数列表                    [刷新] [新建云函数] │
│  🤖  Cloud Agent             │  │ 共 2 个文件 · 当前项目 商城后台                    │
│  📄  日志管理                 │  ├─────────────────────────────────────────────────┤
│  ⚙   设置                    │  │ 名称        导出函数          更新时间    操作    │
│                              │  │ hello.go    [Hello] [Ping]   09-18 21:04 查看 编辑 删除 │
│                              │  │ order.go    [Create]         09-18 18:12 查看 编辑 删除 │
│                              │  └─────────────────────────────────────────────────┘
└──────────────────────────────┴──────────────────────────────────────────────────────┘
```

侧栏选中态沿用现有 `SidebarMenuButton`：`h-11`（sm `h-9`）、`gap-1.5`、active 为 primary 浅底，不新增颜色 token。

### 8.3 列表页 ASCII（正常 / 空 / admin）

```
  云函数列表                                          [ 刷新 ]  [ + 新建云函数 ]
  共 2 个文件 · 调用前缀  POST /go/{projectId}/{name}/{FunctionName}
┌──────────┬──────────────────────────┬────────────────┬──────────────────────────────┐
│ 文件     │ 导出函数 (ParseFuncList) │ 更新时间       │ 操作                         │
├──────────┼──────────────────────────┼────────────────┼──────────────────────────────┤
│ </>      │  ┌──────┐ ┌──────┐       │                │ [查看] [编辑] [复制路径] [删除]│
│ hello.go │  │Hello │ │Ping  │       │ 2026-09-18     │                              │
│          │  └──────┘ └──────┘       │ 21:04          │                              │
├──────────┼──────────────────────────┼────────────────┼──────────────────────────────┤
│ </>      │  ┌────────┐              │ 2026-09-18     │ …                            │
│ order.go │  │Create  │              │ 18:12          │                              │
│          │  └────────┘              │                │                              │
└──────────┴──────────────────────────┴────────────────┴──────────────────────────────┘
  共 2 条                                      < 1 / 1 >
```

- 文件列：`CodeIcon` + `sb-mono` 展示 `{name}.go`；tooltip 显示无后缀 `name`。
- 导出函数列：每个 Name 一枚 `Badge variant="secondary"`，等宽；**点击 Badge 复制**完整调用 URL（toast「已复制 POST /go/…/Hello」）。无导出（理论上保存拦掉）显示 `—`。
- 操作：「查看」只读打开同一 modal；「编辑」可写；「删除」走 `ConfirmAction`（「确认删除 hello.go？已导出的 Hello / Ping 将立即不可调用。」）。
- 工具栏副文案用 `text-xs text-muted-foreground` 写出调用前缀，避免用户猜路径。

空态：

```
┌─────────────────────────────────────────────────────────────┐
│                                                             │
│              还没有云函数                                   │
│     新建一个 .go 文件，导出大写函数后即可 HTTP 调用           │
│                                                             │
│                   [ + 新建云函数 ]                          │
└─────────────────────────────────────────────────────────────┘
```

admin 项目：隐藏「新建」，空态文案「系统项目不支持云函数」，无 CTA。

### 8.4 统一 Modal ASCII（新建 / 编辑 / 查看）

宽度走现有 `SbModal` 的 `width >= 900` → `sm:max-w-5xl`。编辑器最小高度 **420px**，弹窗内容区 `min-w-0` 避免 Monaco 把布局撑破。

```
┌─ 新建云函数 / 编辑 hello.go / 查看 hello.go ───────────────────────── [x] ─┐
│                                                                           │
│  云函数名  [ hello        ] .go     （编辑/查看时 Input disabled）         │
│                                                                           │
│  ┌─ Alert ──────────────────────────────────────────────────────────────┐ │
│  │ HTTP+JSON：导出函数必须是 func Name(req T) R（1 入参 / 1 返回值）。     │ │
│  │ 调用 POST /go/0000-…-0002/hello/{Name}  ·  Body→req  ·  响应=R 的 JSON │ │
│  │ 请使用大写函数名，否则不会出现在列表、也无法调用。                      │ │
│  └──────────────────────────────────────────────────────────────────────┘ │
│                                                                           │
│  将导出  [Hello] [Ping]     ← 前端用正则预览；保存以服务端 ParseFuncList 为准│
│                                                                           │
│  ┌─ Monaco (language=go, 暖色 vs 定制) ─────────────────────────────────┐ │
│  │  1  package main                                                     │ │
│  │  2                                                                   │ │
│  │  3  type Request struct {                                            │ │
│  │  4      Name string `json:"name"`                                    │ │
│  │  5  }                                                                │ │
│  │  …  func Hello(req Request) Response { … }                           │ │
│  │                                                                      │ │
│  └──────────────────────────────────────────────────────────────────────┘ │
│                                                                           │
│                              [ 取消 ]     [ 保存 ]   ← 查看模式仅 [关闭]    │
└───────────────────────────────────────────────────────────────────────────┘
```

三种 mode 共用一个组件：

| mode | 标题 | 名称 | 编辑器 | 页脚 |
|---|---|---|---|---|
| create | 新建云函数 | 可编辑 | 可写，预填模板 | 取消 / 保存 |
| edit | 编辑 {name}.go | disabled | 可写 | 取消 / 保存 |
| view | 查看 {name}.go | disabled | `readOnly: true` | 关闭 |

导出预览：客户端 `/^func\s+([A-Z][A-Za-z0-9]*)\s*\(/gm` 只作即时 Badge；**不**当校验。保存失败时 `toast.error` 展示后端 `error.message`（编译错误原文）。

查看/编辑从 `GET :p/gofunctions/:name` 拉 `source`，避免列表接口带大源码。

### 8.5 Monaco（依赖例外）

`ui/AGENTS.md` 默认禁止加依赖；本需求点名 Monaco，允许：

| 包 | 用途 |
|---|---|
| `monaco-editor` | 编辑器本体 |

**不**再加 `@guolao/vue-monaco-editor` 等 Vue 封装。自写 `GoMonacoEditor.vue`：

- `monaco-editor/esm/vs/editor/editor.api` + `editor.worker?worker`
- `MonacoEnvironment.getWorker` **只**返回 editor worker（不用 TS/JSON language worker，减小体积）
- Monaco **无内置 Go**。在 `ui/src/components/editor/goMonarch.ts` 注册 `language: 'go'` 的 Monarch 高亮（关键字、类型、注释、字符串、struct tag），不做 LSP / 补全（`gofunction/importer` 的 completion 已按简化计划删除，不要接）
- 主题：自定义 `sb-light`，颜色贴近现有 token（背景 `--card`/`--popover` 米白、主色不做大面积着色、关键字用 foreground、字符串用偏暖的 secondary）
- `tabSize: 4`，`minimap: { enabled: false }`，`fontFamily` 用现有 `sb-mono` 栈
- `onUnmounted` 必须 `editor.dispose()`
- 查看模式 `updateOptions({ readOnly: true })`

`vite.config.ts` 允许的最小改动：

```ts
optimizeDeps: { include: ['monaco-editor'] },
worker: { format: 'es' },
proxy: { '/v1': …, '/health': …, '/go': … }
```

构建后 `npm run build` 必须通过；Monaco 会增大 chunk，用动态 `import()` 让 `GoFunctions` 路由才加载编辑器。

---

## 9. 前端 services

`types.ts` 增加：

```ts
export interface GoFunctionItem {
  id: string
  name: string
  file: string
  source?: string
  exports: string[]
  createdAt: string
  updatedAt: string
}

export interface GoFunctionCreate {
  name: string
  source: string
}
```

`Api.gofunctions`：

```
list(projectId): Promise<GoFunctionItem[]>
create(projectId, body): Promise<GoFunctionItem>
get(projectId, name): Promise<GoFunctionItem>
update(projectId, name, source): Promise<GoFunctionItem>
remove(projectId, name): Promise<void>
```

`http-api.ts` 路径函数 `gofunctionsPath(projectId, name?)`，一律 `encodeURIComponent`。列表响应解包 `functions`；snake_case → camelCase 集中在 adapter。

`mock.js` 内存数组，种子一条 `hello` + 模板源码 + `exports: ['Hello']`。

页面从 `projectStore` 取 `projectId`，`watch` 切换项目时重载。禁止 services import store。

---

## 10. 安全、配额、可观测

| 项 | 策略 |
|---|---|
| 鉴权 | CRUD 与 `/go` 均 Bearer；跨项目 403 `cross_project_denied` |
| 租户隔离 | `sys_gofunctions.project_id`；invoke 先 `GetGoFunction(projectID, name)` |
| 运行沙箱 | 不是 OS 沙箱。脚本只能碰已注册标准库。`net/http` 可出网（SSRF）——文档写明「信任本项目持 Key 的作者」 |
| 超时 | `RunJSON` 走解释器内置 10s Context（见 §6 要点 7）；超时 504 |
| 并发 | MVP 不单独加槽位；吃全局 `BodyLimit`。若压测打满 CPU，后续用 `limits.max_concurrent_queries` 同类 semaphore |
| 配额 | MVP **不**接 `CheckQuota`（现网 kind 只有 llm/database） |
| 审计 | 写操作（create/update/delete）记 `audit` kind=`gofunction`，**不**把源码写入审计 detail |
| 日志 | invoke 记 method+path+status；错误 message 截断 512 字节 |
| 系统项目 | 拒绝写；读返回空列表 |

### 10.1 Metrics 上报（必做）

复用 `systemdb.RecordMetric`（`MetricSample{ProjectID, Name, Value}`，与 `http_requests` 同通道），在 Invoke handler 与保存路径埋点：

| 指标名 | 时机 | Value | 用途 |
|---|---|---|---|
| `gofunction_invokes` | 每次 invoke（含失败） | 1 | 调用量；Dashboard/日志聚合 |
| `gofunction_invoke_duration_ms` | invoke 成功返回前 | 耗时毫秒（float64） | 性能观测、定位慢函数 |
| `gofunction_invoke_errors` | invoke 返回 4xx/5xx（含超时） | 1 | 错误率；与 invokes 比对 |
| `gofunction_compile_ms` | 保存时 `ValidateHTTPFuncs`（create+update） | 编译+校验耗时毫秒 | 保存体验、编译器健康度 |

实现约定：

1. 埋点位置在 `gofunction_handler.go` 的 `Invoke` / `Create` / `Update` 内，通过 `deps.System.RecordMetric(...)`（与 router.go 现有 `http_requests` 写法一致），**不**在 `gofunction` 包内做上报（依赖方向禁止反向）。
2. `gofunction_invokes` / `errors` 附带 label 信息进 `MetricSample` 不支持，保持纯计数即可（与 `http_requests` 同粒度）。
3. Prometheus 侧无需新增 collector：`http_requests` 已按 route/method/status 打点，`/go/...` 路径自动覆盖；`gofunction_*` 走系统库 rollup（`sys_metric_rollups_hourly`）供 Dashboard「监控大盘」展示。
4. Dashboard（`ui/src/pages/Dashboard.vue`）与 `MetricsSummary`（`internal/api/system_handlers.go`）后续可把 `gofunction_invokes` 纳入卡片；本计划只保证数据落库，前端展示归 `ui-plan-v2.md` 的 Dashboard 迭代。
5. 上报失败静默（与 `RecordLog`/`RecordMetric` 现有容错一致），不影响请求返回。

---

## 11. 阶段

| 阶段 | 内容 | 产出 |
|---|---|---|
| **A 契约与内核** | `ValidateHTTPFuncs` + `RunJSON` + 测试 | `gofunction/jsonrun.go` |
| **B 存储 + CRUD** | v28 表、store、handler、error 映射、router、writable/admin 守卫 | `internal/systemdb/gofunctions.go`、`internal/api/gofunction_handler.go` |
| **C 调用面** | `POST /go/...`、vite `/go` 代理、指标 | `Invoke` handler |
| **D 控制台** | 路由/侧栏、列表页、Modal、Monaco、mock/http-api | `ui/src/pages/GoFunctions.vue` 等 |
| **E 收口** | `proto-http.md` §3.13、`proto.http`、`docs/gofunction/*.md` + `_meta.json`、`ui-plan-v2` 配套表 | Wiki「云函数」 |

建议合并顺序：A → B → C 可同一 PR；D 可同一 PR 或紧随。E 必须同 PR 否则控制台与契约脱节。

---

## 12. 文件清单（落地时）

**后端**

- `gofunction/jsonrun.go` / `jsonrun_test.go`
- `internal/systemdb/migrate.go`（v28）
- `internal/systemdb/gofunctions.go` / `gofunctions_test.go`
- `internal/api/gofunction_handler.go` / `gofunction_handler_test.go`（含 metrics 埋点）
- `internal/api/router.go`、`error.go`（新 code + `/go` 组挂载与 405）
- `internal/app/app.go`（无需新依赖对象，用已有 `System`；确认 blank import packages）

**前端**

- `ui/src/router/index.ts`、`components/NavMenu.vue`
- `ui/src/pages/GoFunctions.vue`
- `ui/src/components/modal/GoFunctionModal.vue`
- `ui/src/components/editor/GoMonacoEditor.vue`、`goMonarch.ts`
- `ui/src/services/types.ts`、`http-api.ts`、`mock.js`
- `ui/package.json`（`monaco-editor`）、`ui/vite.config.ts`（proxy + worker）

**文档 / 契约**

- `plan/planv2.0/proto-http.md`、`proto.http`
- `docs/_meta.json` 增模块 `gofunction`；`docs/gofunction/overview.md`、`invoke.md`
- 本文件状态改为 implemented（落地后）

---

## 13. 错误码（追加到 `error.go` / proto）

| HTTP | code | 场景 |
|---|---|---|
| 400 | `invalid_gofunction_name` | name 不符合 `^[A-Za-z][A-Za-z0-9_]{0,62}$` |
| 400 | `gofunction_compile_error` | `BuildProgram` 失败 |
| 400 | `gofunction_signature_invalid` | 导出函数不是 1 入参 1 返回，或零个导出 |
| 400 | `gofunction_source_too_large` | 超过 256KiB |
| 409 | `gofunction_already_exists` | 同项目同名未归档 |
| 404 | `gofunction_not_found` | 文件不存在或已归档 |
| 404 | `function_not_found` | 文件在、函数不在（或未导出） |
| 405 | `method_not_allowed` | GET `/go/...`（防 SPA fallback 吞掉） |
| 422 | `gofunction_limit_exceeded` | 每项目超过 100 个 |
| 500 | `gofunction_runtime_error` | 解释执行失败 |
| 500 | `gofunction_compile_error` | invoke 时 `BuildProgram` 失败（保存后环境变化） |

---

## 14. 验收（G1–G12）

| # | 标准 |
|---|---|
| G1 | 侧栏有「云函数」，点开右侧为当前项目的文件列表；顺序在 S3 与 Cloud Agent 之间 |
| G2 | 可新建：modal 预填模板 + 固定 HTTP+JSON 提示；保存后列表出现 `{name}.go` 与导出 Name Badge |
| G3 | 查看 / 编辑共用 modal；查看只读；编辑改源码后 exports 更新 |
| G4 | 删除需确认，确认后列表消失，invoke 404 |
| G5 | Badge /「复制路径」得到 `POST /go/{projectId}/{name}/{FunctionName}` |
| G6 | `curl -H "Authorization: Bearer …" -H "Content-Type: application/json" -d '{"name":"a"}' /go/{pid}/hello/Hello` 返回 `{"message":"hello, a"}` |
| G7 | 保存 `func Bad(a, b int) int` 或小写 `func hello` 仅有未导出函数 → 400 且 message 指明函数名；struct 含未导出字段 → 400；列表不出现错误文件 |
| G8 | 未登录 / 错 Key 调 `/go` → 401；其它项目资源 → 403/404 |
| G9 | admin 项目无新建；Mock 与 http 双实现签名一致；`yarn/npm run build` 与 `go test ./gofunction/... ./internal/...` 通过 |
| G10 | Wiki `docs/gofunction` 可从顶栏「使用文档」打开；`proto.http` 含 CRUD + invoke 样例 |
| G11 | 浏览器 GET `/go/{pid}/hello/Hello` 返回 405 JSON（非 index.html） |
| G12 | invoke 后 `sys_metric_samples` 出现 `gofunction_invokes` / `gofunction_invoke_duration_ms`；失败请求出现 `gofunction_invoke_errors`；保存后出现 `gofunction_compile_ms` |

---

## 15. 风险

| 风险 | 缓解 |
|---|---|
| 每次 invoke 都 `BuildProgram`，冷路径慢 | MVP 接受（不做缓存）。**升级警告**：`Program.globals` 共享同一 cell，并发 `Run` 同一 Program 时脚本级全局变量写入构成 data race（`-race` 必炸）。必须先解决全局变量按请求隔离，才允许按 `(projectID, name, updated_at)` 缓存 `*Program`；缓存命中也要按请求 `Run`，不得复用 frame |
| 用户 struct 与 `encoding/json` 的 tag/导出字段 | 模板与 Alert 写明；`ValidateHTTPFuncs` 保存时 typeChange 预演拦截（§6），绑定失败 400 |
| Monaco 打包体积 | 路由级动态 import；只用 editor worker |
| `net/http` SSRF | 文档 + 仅持 Key 者可改源码；后续可从 packages 摘掉 http 或加 allowlist（非本计划） |
| 解释器不是完整 Go | 文档列出已注册包；编译错误把 go/types 的 message 回给用户 |
| SPA GET fallback 吞掉误用的 GET `/go` | **必做**：同步注册 GET 返回 405 JSON（§7.2），不再作为可选项 |
| 解释器超时 error 包装导致 504 失配 | `RunJSON` 内识别 DeadlineExceeded（§6 要点 7），测试覆盖死循环脚本 |

GET `/go/:projectID/:name/:functionName` **必须同步注册**，返回 **405** + `{"error":{"code":"method_not_allowed","message":"use POST with JSON body"}}`，避免踩 SPA HTML（验收 G11）。
