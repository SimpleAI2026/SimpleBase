# gofunction 重构与修复计划（GoFunction Refactor & Fix Plan）v2

> **日期**：2026-09-18（v2，基于全量代码审阅 + 实际编译验证修订）
> **范围**：`gofunction/`（约 20 个源文件 + 12 个测试文件，两套不同来源的代码碎片）
> **结论先行**：当前 `gofunction` **完全无法编译**（`go build` 直接失败，6 个 import 指向不存在的包）。目录内是**两套 API 互不兼容的代码碎片拼接**，顶层解释器依赖的支撑包缺失，需要「重建地基 + 删半成品 + 修 bug」三线并行。

---

## 1. 现状分析（已逐文件核实）

### 1.1 定位

`gofunction` 是一个 **Go 语言子集的 SSA 解释执行引擎**：将 Go 源码经 `golang.org/x/tools/go/ssa` 编译为 SSA，再用 `reflect` 逐指令解释执行，从而在不调用 `go build` 的情况下运行用户脚本，并支持向脚本注入宿主函数（`importer.Registry`）。核心链路：

```
Run ─→ BuildProgram(parser→ssautil.BuildPackage) ─→ Program.Run
     ─→ callSSA ─→ runFrame ─→ visitInstr ─→ runXXX（instruction.go）
     ─→ ops.go 适配层 ─→ internal/operations（算术/比较/调用）
     ─→ internal/importer（外部包注册与导入）+ internal/value（值抽象）
```

### 1.2 关键事实：两套碎片拼接（v2 修订，最重要）

目录里的代码来自**两个不同的项目**，API 互不兼容，谁也不能直接用谁的：

| | 碎片 A（顶层 `package gofun`） | 碎片 B（`internal/` 子包） |
|---|---|---|
| 来源 module | `git.woa.com/vasd_masc_ba/YitihuaOteam/base/gofun` | `github.com/linkxzhou/webgo` |
| 文件 | `call.go`、`instruction.go`、`frame.go`、`interpreter.go`、`ops.go`、`type.go`、`runtime.go`、`executor.go`、`executor_pool.go`、`instruction_handler.go` | `internal/{value,importer,operations,instructions}/`、`internal/{optimized_instruction,example}.go` |
| 完成度 | **成熟的解释器主体**（`visitInstr` 覆盖全部 30+ SSA 指令），但**依赖的支撑包不在目录里** | 半成品：`instructions/` 大量「简化实现」（`handleGo` 只打日志、`make` 返回空值）；`operations` 完整可用；`value` 的 API 与顶层期望**不兼容** |

**碎片 A 期望的 `value` 包 API**（顶层代码实际调用点反推）：`value.RValue{Value: v}`（**值类型**复合字面量）、`value.ValueOf`、`value.Package`/`value.Unpackage`（多返回值打包）、`value.MapIter{I, Value, Keys}`、`value.ExternalValue`（含 `ToValue`/`Store`）、`value.ExternalValueWrap(importer, mainPkg)`。

**碎片 B 实际提供的 `internal/value`**：`ReflectValue`（**指针类型**，`NewReflectValue` 返回 `*ReflectValue`，且各方法带 panic 检查）、`MapIterator`（字段名完全不同）、`NewReflectValue`。**没有** `RValue`/`Package`/`Unpackage`/`MapIter`/`ExternalValue`/`ExternalValueWrap`。

→ 直接把 `internal/value` 提升为 `gofunction/value` 并不能让顶层编译通过，**必须按碎片 A 的调用点重建 value 包**。

**碎片 A 期望的 `packages` 包**：`gofun_test.go` 有 `_ ".../gofun/packages"`（blank import 触发标准库注册），`packages_test.go` 的 12 个测试全部依赖它注册 `fmt/strings/math/time/regexp/encoding/json/encoding/base64/bytes/sync/atomic/net/http/io/ioutil`。**该包完全缺失**。

**碎片 A 依赖的腾讯内部库**（目录里不存在，主工程也没有）：`git.woa.com/.../base/{log,runtime,trace,snowflake}`；顶层还调用了 `console(...)`（`call.go:115`、`runtime.go:100`）——**该函数在整个目录中无定义**。

### 1.3 编译验证结果（2026-09-18 实测）

```
$ cd gofunction && go build ./...
interpreter.go:15: no required module provides package git.woa.com/.../gofun/internal/importer
call.go:11:      no required module provides package git.woa.com/.../gofun/internal/value
call.go:12:      no required module provides package git.woa.com/.../base/log
call.go:13:      no required module provides package git.woa.com/.../base/runtime
runtime.go:8:    no required module provides package git.woa.com/.../base/snowflake
executor.go:8:   no required module provides package git.woa.com/.../base/trace
（另：go.sum 缺 x/sync、x/mod 条目；x/tools v0.35.0 与主工程 v0.41.0 不一致）
```

顶层 10 个文件中 7 个直接编译失败；`internal/` 下碎片 B 自身可编译（模块名 webgo），但与顶层拼不起来。

### 1.4 依赖版本对比（问题 1）

| 项 | 主工程 `go.mod` | `gofunction/go.mod` |
|---|---|---|
| module | `github.com/linkxzhou/SimpleBase` | `github.com/linkxzhou/webgo`（与目录归属不符） |
| go | `1.25.0` | `1.24.4` |
| `golang.org/x/tools` | `v0.41.0`（indirect） | `v0.35.0` |

---

## 2. 问题清单

### A. 编译级问题（阻断性）

| # | 位置 | 问题 |
|---|---|---|
| A1 | 7 个顶层文件 | import 指向不存在的 `git.woa.com/...` 包（value/importer/log/runtime/snowflake/trace/packages） |
| A2 | `ops.go` | `frameAdapter` 声称实现 `operations.FrameInterface`，但两个接口里的 `value.Value` 来自**两个不同的包**（碎片 A 的 value vs webgo 的 value），接口不满足，编译失败 |
| A3 | `ops.go:83-88` | `callSSAAdapter` 把 `env interface{}` 直接传给 `callSSA` 的 `env []*value.Value` 参数，类型不匹配 |
| A4 | `instruction_handler.go` | 引用未定义的 `ExecutionFrame` 类型；`handleAlloc/handleStore/handleBinaryOperation/...` 等方法全部未定义（它们在 `internal/instructions/` 里，但那边用的是另一套 `ExecutionFrame`）。与 `internal/instructions/` 是**两份互不连贯的同名半成品**（`MemoryInstructionHandler` 等类型重复定义） |
| A5 | `runtime.go`/`executor.go`/`executor_pool.go`/`call.go` | 引用 `trace.OpentracingObject`、`trace.OpentracingCostTime`、`goruntime.DefaultOptions`、`snowflake.GenerateNextID`、`log.Info/Error/Debug/Init`、`console()` —— 全部不存在 |
| A6 | `gofun_test.go` | 引用 `.../gofun/packages`（缺失）与 `log.Init`；`packages_test.go` 12 个测试因缺标准库注册全部无法通过 |
| A7 | `gofunction/go.mod` | 独立 module 阻断主工程引用；`internal/` 目录语义（仅限同 module 上层引用）与「做成公共库」目标冲突 |

### B. 运行时 bug（编译修复后仍会出错）

| # | 位置 | 问题 |
|---|---|---|
| B1 | `instruction.go runSelect` | `reflect.ValueOf(fr.get(state.Chan))` 把 `value.Value` **接口**包成 reflect.Value（Kind=Struct），`reflect.Select` 必然 panic。应使用 `fr.get(...).RValue()`（`state.Send` 同样） |
| B2 | `executor_pool.go Close()` | 持有 `p.mutex` 时遍历调用 `executor.Close()`，而 `Executor.Close()` 内部又调用 `pool.putExecutor`/`pool.decrementActive`（都要 `p.mutex.Lock`）→ **同 goroutine 重复加锁死锁** |
| B3 | `executor.go Close()` | 过期判断 `time.Since(e.createTime) > e.pool.MaxAge*time.Minute`：`MaxAge` 已是 `time.Duration`，再乘 `time.Minute` 阈值放大 6000 倍（30min→1800 天），且与 `putExecutor` 内的判断（不乘）不一致 |
| B4 | `call.go callBuiltin` | `append`：`args[1].Elem().Len()` 对 slice 值调用 `Elem()`（仅 Ptr/Interface 合法）；`copy` 分支无 return，`copy` 作为表达式时返回错误结果 |
| B5 | `operations/call.go Call` | `callSSA(caller, fun, args, nil)` env 恒为 nil，被调函数有 FreeVars 时 nil 解引用 panic（签名应统一为 `[]*value.Value`） |
| B6 | `interpreter.go GetGlobalValue` | 返回 `v.Type().(*types.Pointer).Elem()`（一个 `types.Type`！）而非全局变量的**值**——语义完全错误（`TestGetGlobalValue` 期望拿到 map 值）；函数注释还错抄成「修改全局变量的值」 |
| B7 | `interpreter.go ParseFuncList` | `switch t := v.(type)` 内又做 `v.(*ast.FuncDecl)` 二次断言，`t` 使用混乱；方法过滤（`Recv != nil`）逻辑藏在冗余断言里 |
| B8 | `operations/call.go GoCall` | goroutine 计数只在调用处增减，`Run` 结束时无人等待归零（`RuntimeAsync.waitGroup` 从未接线）→ `go` 语句的协程可越过 `Run` 生命周期泄漏 |
| B9 | `call.go runFrame` | `case _JUMP: break` 只跳出 switch 不跳出内层循环，靠「If/Jump 必为 block 终结指令」这一 SSA 不变量兜底——脆弱写法，应改为带标签 break |
| B10 | `instruction.go runStore` default 分支 | `v := *fr.env[addr]` 对 env 中不存在的 key 解引用 nil 而 panic；`frame.get` 对 `*ssa.Global` 未初始化时落到 env 查找，报错信息误导 |
| B11 | `call.go visitInstr` debugging 分支 | `v := *fr.env[val]` 可能解引用 nil（值未注册时） |
| B12 | `executor.go Execute` | 把 `functionName` 传给 `BuildProgram(seqid, fname, ...)` 的**文件名**参数——参数语义错位；`Run(..., "exports")` 硬编码入口函数名 |
| B13 | `type.go typeChange` | `*types.Interface` 与 default 分支都返回 `reflect.TypeOf(func(interface{}) {}).In(0)`（即 `interface{}`）：写法晦涩，default 分支会把 Signature/Named 等错误类型静默映射成 `interface{}`，掩盖 bug——应显式报错 |
| B14 | `executor_pool.go GetExecutor` | 池满返回 `nil`，调用方极易漏判 → 建议改为 `(executor, error)`（改进项） |

### C. 风格 / 结构问题（问题 2、3）

1. `package gofun` 与目录名 `gofunction` 不一致；模块名 `webgo` 与归属不符。
2. 两套碎片并存：顶层 `instruction_handler.go`（壳）与 `internal/instructions/`（另一套壳+实现）重复定义同名类型，互不引用。
3. 死代码：`internal/errors/`（无任何引用）、`internal/optimized_instruction.go` 与 `internal/example.go`（import webgo 路径，属碎片 B 孤岛）、`internal/OPTIMIZATION_SUMMARY.md`（描述的是旧路径 `/Volumes/my/github/webgo/internal` 的重构记录）。
4. 迁移痕迹注释大量残留：「重命名自 Vm / Seqid / goExector / GoPool / execTimeout」等。
5. 未遵循主工程 `internal/AGENTS.md` 的分层约束；无法被 `internal/`、`cmd/` 引用（独立 module）。
6. `internal/value` 的 `ReflectValue`（指针返回 + panic 检查风格）与顶层期望的 `RValue`（值类型 + 透传风格）是两种设计，必须二选一（推荐顶层风格，见 3.1）。

---

## 3. 重构方案

### 3.1 总体策略：以碎片 A 为主体，重建地基

碎片 A 的 `visitInstr` 是唯一完整覆盖 SSA 指令集的实现（碎片 B 的 `instructions/` 全是「简化实现」），**以碎片 A 为主体**；碎片 B 中 `operations`（完整可用）与 `importer`（与顶层 API 匹配：`NewImporter`/`GetPackageByName`/`GetExternalType` 均存在）**保留**；碎片 B 的 `value` 按碎片 A 调用点**重建**；碎片 B 的 `instructions`、顶层 `instruction_handler.go`、`optimized_instruction.go`、`example.go`、`errors/` **删除**。

缺失地基按顶层调用点重建：

**`gofunction/value`（重建，约 200 行）**
- `RValue struct { reflect.Value }`（**值类型**，方法直接透传 reflect，不加 panic 检查）
- `ValueOf(interface{}) Value`
- `Package([]reflect.Value) Value` / `Unpackage(Value) []reflect.Value`（多返回值打包为 `[]value.Value` 元组）
- `MapIter struct { I int; Value Value; Keys []reflect.Value }`，`Next() Value` 返回 `(ok, key, val)` 三元组（对应 `ssa.Next`）
- `ExternalValue`（包装注册表对象，`ToValue() Value`、`Store(Value)`）
- `ExternalValueWrap(*importer.Importer, *ssa.Package)`（把 SSA 全局符号绑定到注册表对象）
- 语义约束：`zero(types.Type)` 返回 `reflect.New(...)` 的**指针**——`callSSA`（局部变量地址）与 `constantOps.Evaluate`（`zero(t).Elem()`）两处调用都依赖该语义，重建时不得改变。

**`gofunction/packages`（新建，约 300 行）**
- `init()` 中向 `importer.GlobalRegistry` 注册 `packages_test.go` 所需标准库：`fmt`、`strings`、`math`、`time`、`regexp`、`encoding/json`、`encoding/base64`、`bytes`、`sync/atomic`、`io/ioutil`、`net/http`（函数用 `CreateFunction`、常量用 `CreateConstant`、变量用 `CreateVariable`）
- 覆盖标准库以「够用为准」，按测试用例逐个补齐，不做全集

**外部依赖替换（问题 1 的一部分）**

| 原依赖（腾讯内部） | 替换方案 |
|---|---|
| `log.Info/Error/Debug` | 注入 `*slog.Logger`（主工程 go 1.25，推荐 `log/slog`），默认 `slog.Default()`；顶层调试日志走同一 logger |
| `goruntime.ConsoleLevelDebug` + `console()` | 删除 `console()` 调用，`print/println` 输出统一写入 `Context.outBuffer`（`Context.Output()` 已存在），需要外部观察时由调用方读取 |
| `snowflake.GenerateNextID` | `github.com/google/uuid`（主工程已有依赖）或调用方直接传入 `seqid`（`Run` 本来就收 `seqid` 参数，`Runtime.Initialize` 里自己生成属于职责重复——**改为接受外部传入**） |
| `trace.OpentracingObject`/`OpentracingCostTime` | 删除。耗时统计用 `time.Since`；链路追踪后续按主工程可观测性方案再接 |
| `goruntime.DefaultOptions`（ExecTimeout 等） | `ExecutorPoolConfig` 已有配置结构，改为显式配置参数 |

### 3.2 目标目录结构

```
gofunction/
├── gofun.go            # 公共 API 入口：Run / ParseFuncList / Context（合并 interpreter.go + call.go 的上下文部分）
├── program.go          # Program：BuildProgram / Run / RunWithContext / Set|GetGlobalValue
├── frame.go            # frame 栈帧 + Context（现 frame.go）
├── instruction.go      # visitInstr + runXXX 全量 SSA 指令执行（现 instruction.go，吸收 call.go 的 runFrame/visitInstr）
├── call.go             # callSSA / callBuiltin / callExternal / makeFunc 辅助
├── ops.go              # unop/binop/constValue/callOp/goCall —— 直接调用 operations 包（修复适配层）
├── type.go             # typeChange / conv / zero / deref
├── runtime.go          # Runtime（去 trace/snowflake）
├── executor.go         # Executor / ExecutorPool（修死锁与单位 bug）
├── value/              # 【重建】RValue / Package / Unpackage / MapIter / ExternalValue / ExternalValueWrap
├── importer/           # 【保留改造】Importer / Registry / Object（现 internal/importer，改 import 路径即可）
├── operations/         # 【保留】manager / binop / comparison / unary / call（统一 value 包引用）
├── packages/           # 【新建】标准库注册（fmt/strings/math/time/...）
├── testdata/           # 【保留】对拍测试数据
└── *_test.go           # gofun_test / packages_test 等（修正 import 与断言）
```

**删除清单**：
- `gofunction/go.mod`、`gofunction/go.sum`（并入主 module）
- `gofunction/internal/` 整层打平上移（`internal` 语义与公共库目标冲突）
- `instruction_handler.go`（碎片 B 壳）
- `internal/instructions/`（碎片 B 半成品，含其 README.md）
- `internal/optimized_instruction.go`、`internal/example.go`（碎片 B 孤岛）
- `internal/errors/`（无引用死代码）
- `internal/OPTIMIZATION_SUMMARY.md`（描述失效路径的旧文档）
- `internal/value/`（被重建的 `value/` 取代；其测试文件中有效的用例迁移到新 `value/` 下）

### 3.3 实施步骤（按依赖顺序）

**Step 1：并入主 module（问题 1）**
1. 删除 `gofunction/go.mod`、`go.mod` 对应 `go.sum`。
2. 主 `go.mod` 将 `golang.org/x/tools` 提为 direct 依赖（`go mod tidy` 自动处理版本 v0.41.0）。
3. 全目录 `package gofun` → `package gofunction`，消除目录/包名不一致。

**Step 2：重建 value 地基**
1. 新建 `gofunction/value`，按 3.1 规格实现（RValue 值类型透传风格）。
2. 迁移 `internal/value` 中仍有价值的内容：`DefaultConverter` 保留；`MapIterator`/`SliceIterator` 语义并入 `MapIter`；对应测试迁移改写。

**Step 3：importer 与 packages 落位**
1. `internal/importer` → `gofunction/importer`，仅改 import 路径（API 已兼容顶层）。
2. 新建 `gofunction/packages`，按 `packages_test.go` 清单注册标准库。

**Step 4：接线顶层解释器**
1. 顶层 8 个文件 import 统一为 `github.com/linkxzhou/SimpleBase/gofunction/{value,importer,operations}`。
2. 重写 `ops.go` 适配层：统一 value 包后 `frameAdapter` 天然满足 `operations.FrameInterface`；`callSSA` 回调签名改为 `func(Frame, *ssa.Function, []value.Value, []*value.Value) value.Value`（同时修 B5）。
3. 日志/序号/console 按 3.1 替换表落地。

**Step 5：删除死代码**（按 3.2 删除清单执行）

**Step 6：修复运行时 bug**（B1–B14，逐项对应代码位置）

**Step 7：风格对齐（问题 2）**
1. 清除「重命名自 xxx」迁移注释；统一中文注释（与 `internal/` 现有风格一致）。
2. 错误处理对齐：公共 API 返回 `error`，不 panic 穿越 API 边界（内部解释器 panic 由 `RunWithContext` 的 recover 统一转 error，现有行为保留）。
3. `go vet`、`gofmt` 清零。

**Step 8：测试与验证（问题 4 的验收）**
1. `go build ./gofunction/...`
2. `go test ./gofunction/... -run 'TestAll|TestImportGofun|TestGetGlobalValue|TestRunFunction'`（testdata 对拍 + 包导入 + 全局变量 + 入口函数）
3. `go test ./gofunction/... -run 'TestAtomic|TestBase64|TestBytes|TestFmt|TestMath|TestStrings|TestTime|TestJson|TestRegexp'`（标准库注入；`TestHttp/HttpRequest/HttpsRequest` 依赖外网，改为 `testing.Short()` 跳过或移入独立 tag）
4. `go vet ./gofunction/...`
5. 主工程 `go build ./...` 不回归。

---

## 4. 风险与注意事项

| 风险 | 说明 | 对策 |
|---|---|---|
| value 包重建规格偏差 | 顶层 8 个文件全部依赖 value；若重建的 API 语义（尤其 `zero` 返回指针、`Package/Unpackage` 元组布局、`MapIter.Next` 三元组）与调用点不符，会引入隐蔽运行时错误 | 以「顶层调用点反推」为唯一规格来源；`testdata` 对拍测试（TestAll）作为语义回归网；重建前先把 gofun_test.go 跑通作为里程碑 |
| x/tools v0.35→v0.41 | `ssa` 包 API 变更 | `ssautil.BuildPackage`/`ssa.SanityCheckFunctions` 等接口在 v0.41 无破坏性变更（该包 API 自 2019 起稳定），低风险；Step 8 编译+测试兜底 |
| 标准库注册工作量 | `packages` 包需反射枚举大量函数签名 | 按 `packages_test.go` 实际用到的函数逐个注册，不做全集；预留 `RegisterXXX` 扩展 API |
| `internal/` 语义 | Go 的 internal 仅限同 module 上层引用；库化后子包若留在 `gofunction/internal/` 下，外部模块引入 `gofunction` 主包没问题，但无法直接用其 importer 注册高级特性 | 打平到 `gofunction/{value,importer,operations,packages}`，全部成为可公开引用的子包 |
| 行为兼容 | 删除 `console()`/trace 会改变可观测性行为 | 输出统一进 `Context.Output()`，调用方可订阅；主工程接入时再按其日志规范桥接 |
| Executor 半成品 | `Executor/Runtime/ExecutorPool` 本身是碎片 A 的周边半成品（注释「重命名自」可证），bug 密集（B2/B3/B12） | 保留但降级为「可选高级 API」；核心路径 `Run/BuildProgram/Program` 不依赖它们，主工程可先用核心 API |

---

## 5. 验收标准

- [ ] `gofunction/` 无独立 `go.mod`/`go.sum`，与主工程共用 `go 1.25.0` + `x/tools v0.41.0`
- [ ] 无任何 `git.woa.com/`、`github.com/linkxzhou/webgo/` import 残留（含测试）
- [ ] `package` 名统一为 `gofunction`；无「重命名自」类迁移注释
- [ ] 死代码清零：`internal/` 打平，`instructions/`、`instruction_handler.go`、`optimized_instruction.go`、`example.go`、`errors/`、`OPTIMIZATION_SUMMARY.md` 均删除
- [ ] A1–A7 编译级问题清零：`go build ./gofunction/...` 通过
- [ ] B1–B14 修复并有对应测试（至少：select 场景、池 Close 不死锁、GetGlobalValue 返回值语义、append/copy 正确性）
- [ ] `go vet ./gofunction/...` 无告警
- [ ] 测试通过：testdata 对拍（TestAll）、TestImportGofun、TestGetGlobalValue、TestRunFunction、标准库注入 9 项（TestAtomic…TestRegexp）；外网用例（TestHttp*）标记跳过
- [ ] 主工程 `go build ./...` 无回归；`internal/`、`cmd/` 可 `import "github.com/linkxzhou/SimpleBase/gofunction"` 使用
