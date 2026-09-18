# gofunction

基于 SSA + reflect 的 Go 脚本解释器。将一段 Go 源码经 `golang.org/x/tools/go/ssa` 编译为 SSA，再按指令用反射解释执行，从而在**不调用 `go build` / 不启动子进程**的情况下运行用户脚本，并把宿主（host）函数、变量、常量、类型注入脚本环境。

本包属于主模块 `github.com/linkxzhou/SimpleBase`（Go 1.25），**没有**独立的 `go.mod`。

## 安装与导入

```go
import "github.com/linkxzhou/SimpleBase/gofunction"

// 空白导入，在 init() 中向 importer.GlobalRegistry 注册标准库符号。
// 脚本里要 import fmt / strings / encoding/json 等，必须先 blank-import 本包。
import _ "github.com/linkxzhou/SimpleBase/gofunction/packages"
```

只跑纯语言特性（算术、`if`/`for`、slice/map、channel、闭包等、不 `import` 外部包）时可以不导入 `packages`。只要脚本引用了已注册包（含 `autoImport` 自动补全的包名），就必须 blank-import `packages`（或自行 `importer.RegisterPackage`）。

## 快速开始

```go
package main

import (
	"fmt"
	"log"

	"github.com/linkxzhou/SimpleBase/gofunction"
)

func main() {
	src := `package main
func add(a, b int) int { return a + b }
`
	result, err := gofunction.Run("seq-1", src, "add", 1, 2)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result) // 3
}
```

完整可运行示例见 [`examples/hello`](examples/hello)。

## 工作原理（简图）

```
Run / Executor.Execute
  └─ BuildProgram
       parser.ParseFile → autoImport（按未解析标识符补 import）
       ssautil.BuildPackage（Importer = importer.NewImporter）
       运行包级 init
  └─ Program.Run / RunWithContext
       callSSA → runFrame → visitInstr（SSA 指令）
       外部函数走 callExternal（reflect.Call 已注册的宿主实现）
```

`seqid` 只是调用方传入的序列号（请求 ID 等），解释器本身不强制格式；空字符串也可以。`BuildProgram` 在 `Executor` 内部若未设置则会生成 `seq-` + 随机 hex。

---

## 公共 API（均来自源码，未编造）

### 编译与执行

| API | 说明 |
| --- | --- |
| `Run(seqid, sourceCode, funcName string, params ...interface{}) (interface{}, error)` | 编译整个源文件并执行名为 `funcName` 的函数。内部即 `BuildProgram` + `Program.Run`。 |
| `BuildProgram(seqid, fname, sourceCode string, packages ...*ssa.Package) (*Program, error)` | 把**单个**源文件编成 `*Program`。`fname` 用作解析文件名（实际为 `fname+".go"`）。可变参数 `packages` 是其它已编译脚本的 `*ssa.Package`（跨 Program 调用），**不是**宿主标准库。 |
| `(*Program).Run(seqid, funcName string, params ...interface{}) (interface{}, error)` | 执行已编译程序中的函数。 |
| `(*Program).RunWithContext(...) (result interface{}, ctx *Context, err error)` | 同上，并返回执行上下文。`print`/`println` 的输出在 `ctx.Output()`。内部 panic 会被 recover 成 `error`。 |
| `(*Program).Package() *ssa.Package` | 取出 SSA 包，可再传给另一次 `BuildProgram(..., pkg)`。 |
| `ParseFuncList(sourceCode string, exportedAll bool) ([]string, error)` | 只做 `go/parser` 扫描，**不编译、不执行**。`exportedAll==false`：仅包级导出函数（跳过方法、`init`、未导出函数）。`exportedAll==true`：所有 `func` 声明（含方法与未导出函数）。 |

源码必须是合法 Go 文件（含 `package` 子句）。入口函数名必须在编译后的包成员中存在，否则 `Run` 返回 `"function not found"`。未导出函数只要你知道名字也可以 `Run`。

参数 `params` 按函数形参顺序传入，经 `value.ValueOf` 装箱。多返回值会按解释器的元组规则打包后 `Interface()` 还原。

### 全局变量

SSA 全局符号类型为 `*T`（单元指针）。下列方法读写的是**单元里的值**，不是指针本身。

| API | 说明 |
| --- | --- |
| `(*Program).GetGlobalValue(name string) (interface{}, error)` | 读包级变量。不存在则报 `global Value %s not found`。 |
| `(*Program).SetGlobalValue(name string, val interface{}) error` | 写包级变量；类型需可赋值或可转换。 |

示例：[`examples/globals`](examples/globals)。须在 `BuildProgram`（会跑包级 `init`）之后、`Run` 之前调用 `SetGlobalValue`，后续执行能读到新值。

### 日志与调试输出

| API | 说明 |
| --- | --- |
| `SetLogger(l *slog.Logger)` | 注入内部 `*slog.Logger`（默认 `slog.Default()`）。`l == nil` 时为 no-op。用于 `Executor.Interrupt` 日志、`Runtime.ConsoleLog`、内部 `logDebug`。 |
| `SetDebugOutput(w io.Writer)` | 设置 SSA 转储等调试输出目标（默认 `os.Stderr`）。`w == nil` 时为 no-op。 |

包内还有未导出的 `debugging` 开关：打开时 `RunWithContext` 会把入口函数 SSA 写到 `SetDebugOutput` 的 writer，并在每条指令后打 debug 日志。**公共 API 没有打开该开关的入口**（测试代码在同包内直接赋值）。因此业务代码通常只需 `SetLogger`。

### Context

`Program.RunWithContext` 返回的 `*Context` 嵌入 `context.Context`（默认 10 秒超时），并提供：

- `Output() string`：脚本里 `print` / `println` 写入的缓冲区。只用 `Run` 时拿不到这块输出。

脚本内 `go` 语句启动的 goroutine 会在返回前等待，最长 `defaultTimeout`（10s）。

### Executor / ExecutorPool

这是可选的对象池封装，核心路径 `Run` / `BuildProgram` **不依赖**它们。

```go
pool := gofunction.NewDefaultExecutorPool() // MaxIdle=10, MaxActive=100, MaxAge=30m
// 或 gofunction.NewExecutorPool(gofunction.ExecutorPoolConfig{...})
defer pool.Close()

ex, err := pool.GetExecutor()
// ...
result, err := ex.Execute("answer", script) // 每次都会 BuildProgram；不接收函数参数
ex.Close() // 放回池中（过期则销毁）
```

| 类型 / API | 说明 |
| --- | --- |
| `ExecutorPoolConfig` | `MaxIdle`、`MaxActive`、`MaxAge time.Duration` |
| `DefaultExecutorPoolConfig` | 见上 |
| `NewExecutorPool` / `NewDefaultExecutorPool` | 建池 |
| `(*ExecutorPool).GetExecutor() (*Executor, error)` | 池关闭 → `ErrPoolClosed`；达到 `MaxActive` → `ErrPoolExhausted` |
| `(*ExecutorPool).Close()` | 关闭池并清理空闲执行器（可重复调用） |
| `(*ExecutorPool).GetStats() (active, idle int)` | 活跃 / 空闲计数 |
| `(*ExecutorPool).SetServiceName` / `GetServiceName` | 调用方标签，解释器不用它 |
| `NewExecutor(pool *ExecutorPool)` | 也可脱离池直接 `NewExecutor(nil)` |
| `(*Executor).Execute(functionName, script string) (interface{}, error)` | 编译并执行**无参**函数（没有 `params`） |
| `(*Executor).Close()` | 有 pool 则 `putExecutor` 或因 `MaxAge` 销毁 |
| `SetSequenceID` / `GetSequenceID` | 传给 `BuildProgram` 的 seqid |
| `SetExecutionTimeout(ms int64)` | `>= 0` 时 `Execute` 会启动 `time.AfterFunc` 调 `Interrupt`；`-1` 表示不启动定时器 |
| `GetCostTime() int64` | 最近一次 `Execute` 耗时（微秒） |
| `Interrupt` / `ClearInterrupt` / `IsInterrupted` | 设置/清除原子标志并打日志。`Interrupt` **不会**取消正在跑的 SSA 循环；循环取消靠 `Context` 超时（且仅在内部 `debugging==false` 时检查） |
| `StartTimeoutTimer` / `StopTimeoutTimer` | 与上面的超时定时器配合 |

下列是**预留空实现**，调用无效果，不要当成已实现功能：

- `(*Executor).Compile(...) error` → 恒 `nil`
- `(*Executor).SetProfile` / `GetProfile` → no-op / `nil`
- `(*Executor).SetWaitEventLoop` → 只存字段，`Execute` 不读取
- `(*Runtime).Cleanup()` → 空函数

`Runtime`（`NewRuntime`、`Initialize`、`Get/SetUserData`、`ConsoleLog`、`Reset`）是 Executor 内部用的运行时对象，也可单独构造；`ConsoleLog` 走 `SetLogger` 的 logger。

示例：[`examples/pool`](examples/pool)。

### 自定义宿主注册（importer）

`BuildProgram` **始终**使用 `importer.NewImporter` → `GlobalRegistry`。没有把自定义 `*Registry` 传进 `BuildProgram` 的参数。因此注入宿主 API 的方式是：在编译**之前**调用全局 `importer.RegisterPackage`。

```go
import "github.com/linkxzhou/SimpleBase/gofunction/importer"

err := importer.RegisterPackage("example.com/host", "host",
	importer.CreateFunction("Add", func(a, b int) int { return a + b }, "a+b"),
	importer.CreateConstant("Version", "v1", ""),
	// importer.CreateVariable(name, valueAddr, reflectType, doc)
	// importer.CreateType(name, reflect.Type, doc)
)
```

脚本：

```go
package main
import "example.com/host"
func demo() int { return host.Add(20, 22) }
```

要点：

- `RegisterPackage(path, name, objects...)`：`path` 对应脚本 `import` 路径，`name` 对应包名。默认不允许重复注册（返回 `package %s already registered`）。
- `CreateFunction` / `CreateVariable` / `CreateConstant` / `CreateType` 分别对应函数、变量（传**地址**）、常量、类型。
- 方法通过 `reflect.Type` 上的 exported method 自动暴露（例如 `base64.StdEncoding.EncodeToString`）。
- 也可先 `BuildProgram` 一个脚本，再把 `p.Package()` 传给另一次 `BuildProgram`，实现脚本间 import（见测试 `TestImportGofun`）。这与宿主 `RegisterPackage` 是两条路径。

相关 API（均在 `gofunction/importer`）：`GetAllPackages`、`GetPackageByName`、`NewRegistry` / `NewRegistryWithConfig`、`GlobalRegistry`、`NewImporter` / `NewImporterWithRegistry`。`NewImporterWithRegistry` 供自定义 `Importer` 使用；**`BuildProgram` 不会调用它**。

`importer` 还提供补全辅助（`GetCompletionItems`、`GetPackageCompletions`、`GetFunctionSignature`、`Keywords`），给编辑器场景用，不参与执行。

示例：[`examples/hostfn`](examples/hostfn)。

---

## 脚本语言能力

解释器覆盖 `visitInstr` 中的 SSA 指令，对应 Go 子集大致包括：

- 基本类型与运算、指针、类型转换、`make` slice/map/chan
- 结构体字段、切片/下标、`range`、`map` 读写与 comma-ok
- `if` / `for` / `switch`、`defer`、闭包（含自由变量）
- `go`、channel send/recv、`select`
- `panic` / `recover`、类型断言、接口
- 内建：`append`、`copy`、`close`、`delete`、`len`、`cap`、`print`、`println`、`panic`、`recover`

`print`/`println` 写入 `Context` 缓冲区，**不会**自动打到宿主 stdout。

`BuildProgram` 的 `autoImport`：若源码里出现未解析标识符，且该名字等于某个已注册包的 **Name**（如 `fmt`），会自动补上对应 `import`。已手写 import 的路径不会重复添加。

---

## 限制（按实现，不是完整 Go）

1. **不是** `go build`。不能链接任意 Go 模块，只能调用已注册（或已作为 `*ssa.Package` 传入）的符号。
2. **标准库不是全集**。`packages` 只注册下面清单中的函数/常量/变量/类型；未列出的（如 `fmt.Fprintf`、`strings.Builder`、`json.NewDecoder`、`http.ListenAndServe`）在类型检查阶段可能以空包/缺符号失败，或运行期 `no implementation for external function`。
3. **单文件编译**。`BuildProgram` 只 `ParseFile` 一次，没有 `go.mod`、build tag、`//go:embed`、cgo、汇编。
4. **宿主注册走全局表**。`RegisterPackage` 默认不可重复；测试 / 多例程共用进程时要注意路径唯一。
5. **`complex` 常量**：`Importer.newObject` 对 `Complex64/128` 常量有 TODO，暂未生成 const value。
6. **`Executor.Execute` 不能传参**。要传参请用 `Run` / `Program.Run`。
7. **每次 `Execute` 都会重新 `BuildProgram`**。池复用的是 `Executor` 对象，不是编译缓存（`functionChecksums` 字段目前未参与执行）。
8. **`Interrupt` 不打断 SSA 循环**；真正停靠默认 10s 的 context 超时。
9. **外网 HTTP**：`net/http` 的 `Get`/`Post` 等是真的发请求。仓库测试里 `TestHttpRequest` / `TestHttpsRequest` 在 `testing.Short()` 下跳过。本地示例用 `net/http/httptest`，不依赖外网。
10. 未注册 path 仍会被 `Importer.Package` 建成**空** `types.Package`（`MarkComplete`），编译期不一定立刻失败，调用其函数会在运行期 panic 并被 `RunWithContext` 收成 error。
11. `Executor.Compile`、`SetProfile`、`Runtime.Cleanup` 等为空实现。

---

## 已注册包清单

由 `gofunction/packages` 的 `init()` 调用 `importer.RegisterPackage`。blank-import 后可用。覆盖原则是「测试与常见脚本够用」，不是标准库全集。

| import path | 包名 | 已注册对象 |
| --- | --- | --- |
| `fmt` | `fmt` | 函数：`Sprintf` `Sprint` `Sprintln` `Printf` `Println` `Print` `Errorf` |
| `strings` | `strings` | 函数：`ToUpper` `ToLower` `Contains` `HasPrefix` `HasSuffix` `Split` `Join` `Replace` `ReplaceAll` `TrimSpace` `Index` `Repeat` |
| `math` | `math` | 函数：`Sqrt` `Abs` `Floor` `Ceil` `Round` `Max` `Min` `Pow` `Mod` `Trunc` `Log` `Log10` `Log2` `Exp` `Sin` `Cos` `Tan`；常量：`Pi` `E` `Ln10` `Ln2` `Sqrt2` `MaxInt` `MinInt` |
| `time` | `time` | 类型：`Time` `Duration` `Location` `Weekday`；函数：`Now` `LoadLocation` `Date` `Parse` `Since` `Unix`；常量：`Second` `Millisecond` `Microsecond` `Nanosecond` `Minute` `Hour` `Saturday` `Sunday` `Monday` |
| `regexp` | `regexp` | 类型：`Regexp`；函数：`MatchString` `QuoteMeta` `Compile` `MustCompile`（`*Regexp` 方法经反射可用） |
| `encoding/json` | `json` | 函数：`Marshal` `Unmarshal` `Valid` |
| `encoding/base64` | `base64` | 类型：`Encoding`；变量：`StdEncoding` `URLEncoding` `RawStdEncoding` `RawURLEncoding` |
| `bytes` | `bytes` | 函数：`NewReader` `Compare` `Equal` `Contains` `Join` |
| `net/http` | `http` | 函数：`Get` `Post` `PostForm` `Head` `NewRequest`；常量：`MethodGet` `MethodPost` `MethodPut` `MethodDelete`；变量：`DefaultClient`；类型：`Client` `Request` `Response` `Header` |
| `errors` | `errors` | 函数：`New` `Is` `As` `Unwrap` |
| `strconv` | `strconv` | 函数：`Itoa` `Atoi` `ParseInt` `ParseFloat` `ParseBool` `FormatInt` `FormatFloat` |
| `sync/atomic` | `atomic` | 函数：`AddInt32` `AddInt64` `AddUint32` `AddUint64` `LoadInt32` `LoadInt64` `LoadUint32` `LoadUint64` `StoreInt32` `StoreInt64` `StoreUint32` `StoreUint64` `CompareAndSwapInt32` `CompareAndSwapInt64` |
| `io` | `io` | 函数：`ReadAll` `Copy` `NopCloser`；变量：`EOF`；类型：`Reader` `Writer` `Closer` `ReadCloser` |
| `io/ioutil` | `ioutil` | 函数：`ReadAll` `ReadFile` `NopCloser` `WriteFile` |
| `encoding/binary` | `binary` | 函数：`Read` `Write` `Size`；变量：`BigEndian` `LittleEndian` |
| `github.com/json-iterator/go` | `jsoniter` | 函数：`Get` `Marshal` `Unmarshal`；类型：`API` `Any` |

运行时可用 `importer.GetAllPackages()` 查看当前进程已注册的 path。

---

## 如何测试

在仓库根目录（模块根）执行：

```bash
# 推荐：跳过访问外网的 TestHttpRequest / TestHttpsRequest，以及 testdata 中的 NilInterface
go test ./gofunction/... -short

# 全量（需要出网；HTTP 用例会请求 qq.com）
go test ./gofunction/...

go vet ./gofunction/...
go build ./gofunction/...
```

`testdata/` 是对拍集：`TestAll` 用解释器跑导出函数，再与 `testdata.TestSet` 同名方法的原生 Go 结果 `reflect.DeepEqual`。覆盖算术、全局变量、import、方法、控制流等。

覆盖率（与重构 PR 相同的命令）：

```bash
go test ./gofunction/... -coverprofile=cov.out -short && go tool cover -func=cov.out
```

跑示例：

```bash
go run ./gofunction/examples/hello
# 或一次性：
for d in gofunction/examples/*/; do echo "== $d"; go run ./$d; done
```

---

## 示例

每个目录都是独立的 `package main`（`main.go` + 短 README），在模块根 `go run ./gofunction/examples/<name>` 即可。

| 目录 | 演示 |
| --- | --- |
| [`examples/hello`](examples/hello) | `Run` 编译执行 `add` |
| [`examples/stdlib`](examples/stdlib) | blank-import `packages`，脚本使用 `fmt` / `strings` / `encoding/json` |
| [`examples/globals`](examples/globals) | `BuildProgram` + `SetGlobalValue` / `GetGlobalValue` |
| [`examples/funclist`](examples/funclist) | `ParseFuncList`（导出函数 vs 全部函数） |
| [`examples/hostfn`](examples/hostfn) | `importer.RegisterPackage` 注入宿主函数 |
| [`examples/pool`](examples/pool) | `ExecutorPool` 取出 / `Execute` / `Close` 归还再取 |
| [`examples/controlflow`](examples/controlflow) | `if` / `for` / `switch` / `defer` |
| [`examples/http`](examples/http) | 宿主 `httptest` + 脚本 `http.Get`（不访问公网） |

子包源码：

- [`gofunction/importer`](importer) — 包注册与 `go/types` Importer
- [`gofunction/packages`](packages) — 标准库注册（blank-import）
- [`gofunction/value`](value) — 解释器值抽象
- [`gofunction/operations`](operations) — 算术 / 比较 / 调用

`gofunction/operations` 与 `gofunction/value` 主要给解释器内部使用，宿主集成一般只碰 `gofunction` + `gofunction/importer` + `gofunction/packages`。
