# gofunction 性能优化方案

- 日期：2026-09-18
- 前置：`gofunction/bench_report.md`（基线数据与剖析结论）
- 现状：解释执行比原生 Go 慢 3～4 个数量级（Fib18 慢 ~1,576 倍，算术循环慢 ~8,555 倍），
  内存分配一半以上来自 `types.TypeString` 字符串化，本质是「每条指令结果都装箱 + 反射转换」。
- 目标：不改对外 API 与语义；第一阶段预期热点场景 3～10 倍提速、分配减少 50%+；
  长期方案给出路线但不阻塞第一阶段。

> **进度（2026-09-18）**：P0-1/2/3、P1-1/2/3/4 已全部落地并通过
> `go build` / `go vet` / `go test -race ./...`，实际收益 1.9~3.6x、分配 -50~82%。
> P2-1/P2-2（预编译分发表 + If/Jump 直通）与 P2-4（Interrupt 真正生效）
> 已于 2026-09-19 落地，visitInstr 分发退出 CPU profile；详见
> `gofunction/bench_report.md`。P2-3（typed value 体系）暂缓，按剩余
> 热点（frame.set 装箱、bool 装箱）评估性价比后再启动。

## 0. 原则

1. 每步优化单独提交，跑 `go build / go vet / go test -race ./...` + 基准对比，无回归才合入。
2. 优化前后用同一命令记录基准：`go test -run='^$' -bench=. -benchmem -count=3 .`。
3. 语义优先：任何加速不得改变 panic/recover/defer/goroutine 语义；有疑义的用现有
   86 个测试用例兜底（program_test/call_test/…/testdata_test）。
4. 分阶段推进：P0 缓存层零风险先行；P1 数据结构改造中等风险；P2 编译期预处理是大改，单独立项。

## 1. P0：消除热路径重复计算（低风险，预期 3～5 倍）

### P0-1 类型映射缓存（替代 types.TypeString 做 map 键）

问题：`typeChange()` 每次先走 `importer.GetExternalType(typ)`，其内部 `goType.String()`
触发 `types.TypeString` 分配（剖析占 alloc 52.6%）。

方案：
- `type.go` 增加包级缓存 `map[types.Type]reflect.Type`（`types.Type` 是接口，
  其动态值指针可安全做 map 键，x/tools interp 也用 types.Type 做 map 键）；
- `typeChange` 先查缓存，miss 时才走原逻辑（含 GetExternalType），结果写回；
- 同理给 `importer.Registry.GetExternalType` 增加 `types.Type` 直接键缓存，
  字符串键仅保留给跨类型查询的注册路径。

验收：`BenchmarkExecFib18` 分配降 ~50%；`BenchmarkBuildProgram` 无退化。

### P0-2 常量求值缓存

问题：`frame.get(*ssa.Const)` 每次都走 `constValue→conv`（剖析累计 35%）。
`*ssa.Const` 在 SSA 中是稳定单例，值恒定。

方案：
- `Program` 增加字段 `constCache map[*ssa.Const]value.Value`，BuildProgram 后惰性填充；
- `frame.get` 命中 `*ssa.Const` 时先查缓存。
- 注意：`zero()` 返回的是可变指针语义值（typed nil / 复合零值），
  仅缓存基础类型常量；复合零值仍每次新建，避免共享可变结构。

验收：含常量循环（ArithLoop/String100）迭代耗时下降。

### P0-3 上下文取消检查降频

问题：主循环每条指令后查 `fr.context.Err()`（context.WithTimeout 的双 map 查询）。

方案：`runFrame` 的 BlockLoop 每完成一个 basic block 检查一次，
指令内不再检查。超时粒度从「每指令」变为「每块」，实际语义不变
（当前 Interrupt 本身也未真正取消 context，仅记日志——见 P2-4 顺带修正）。

验收：`BenchmarkExecArithLoop` 提速；超时相关测试（interp_test）仍通过。

## 2. P1：降低每指令/每调用的固定开销（中风险，累计预期再 2～3 倍）

### P1-1 binop/unop 快路径：免 interface 装箱

问题：`binop` 结果先存 `interface{}`（`var result interface{}` 必然装箱），
再 `conv(result, instr.Type())` 走 `reflect.ValueOf(v).Convert(rtype)`。

方案：
- 结果直接构造 `value.RValue{Value: reflect.ValueOf(具体类型值)}`，
  仅当 `instr.Type()` 与天然结果类型不一致时才 Convert；
- 更进一步：按 `(instr.Op, x.Kind())` 在指令首次执行时记忆 fast-path 函数
  （小型闭包表挂在 frame 或 program 上），后续直接调用。
- 最小改法（推荐先做）：`conv` 增加签名 `convType(v interface{}, want reflect.Type)`，
  `binop` 内已能拿到目标 `reflect.Type`（经 P0-1 缓存后零成本），
  若 `reflect.TypeOf(v) == want` 则直接 `reflect.ValueOf(v)` 包装，跳过 Convert。

验收：ArithLoop / Fib18 显著下降；数值精度测试（eval_test）全绿。

### P1-2 SSA 指令级元信息预解析

问题：`visitInstr` 每次对 `*ssa.BinOp` 做 `instr.Type()`（types 层遍历）+
`typeChange`（即便有缓存也有 map 查询），`runConvert` 每次取 `instr.Type()`。

方案：为热指令引入轻量「预编译缓存」结构：

```go
type instrInfo struct {
    rtype reflect.Type // instr.Type() 解析后的 reflect 类型
    // binop 专用：op 已知、类型已知，可预判 fast path
}
```

挂在 `Program`（并发安全：构建后只读），`frame` 经 `fr.program` 查询；
`map[ssa.Instruction]instrInfo` 或直接在每个 `run*` 首次执行时惰性建立。

验收：与 P1-1 叠加后 Fib18 对比基线累计 ≥5 倍。

### P1-3 函数调用 frame 复用池

问题：每次 `callSSA` 分配 `frame{}`（newChild 7.2% alloc）+
`env` map（make map + 每个参数取址装箱）+ `locals` 切片。

方案（分两步）：
1. `sync.Pool` 复用 `frame` 与其 `env` map（退出 runFrame 时 clear 归还）；
   map 复用需保证 clear 后无悬挂引用（返回前已置 nil locals，风险可控）；
2. 中期：`env` 改为 `map[ssa.Value]*value.Value` → 两级结构
   「params/locals 定长数组（按 SSA 索引）+ 仅堆变量走 map」，
   SSA 的 Params/Locals 本身有序，可直接用切片索引替代大多数 map 查询。
   此改动波及 frame.get/set/runStore 等，作为独立 PR。

验收：ExecCall100 / Fib18 每次调用开销下降 ≥50%；-race 全绿（重点回归 go 语句快照拷贝路径）。

### P1-4 makeFunc 反射包装延迟化

问题：`frame.get(*ssa.Function)` 每次都 `makeFunc` 构建 `reflect.MakeFunc` 闭包
（含 reflect.FuncOf + 数次分配），脚本内直接递归调用 `fib` 也走该路径吗？
——否，`call()` 对 `*ssa.Function` 直接 callSSA，不走 makeFunc；但把函数
赋值给变量/作为参数传递时会反复包装。缓存：`Program.funcCache map[*ssa.Function]value.Value`。

验收：新增基准 `ExecCallIndirect100`（函数值作为参数传递）后对比。

## 3. P2：结构性优化（大改，单独立项评估）

### P2-1 指令分发表编译期化（code-gen / 函数表）

`visitInstr` 的类型 switch 在每个指令 ~30+ case 中线性比较接口动态类型。
方案：构建期把每个 `ssa.BasicBlock.Instrs` 映射为 `[]func(*frame, ssa.Instruction) step`
切片（闭包已绑定具体指令类型），运行期直接调用，消除分发开销。
预期主循环分发成本接近归零；实现工作量中等，收益稳定。

### P2-2 循环内 Phi/If/Jump 直通

对「If → 块尾」模式，将条件求值与块切换融合，减少 stepJump 回外层循环的
开销（外层每轮重建迭代器）。与 P2-1 一并做。

### P2-3 typed value 体系（去 interface{} 化）

`value.Value` 接口 + `reflect.Value` 装箱是根本瓶颈：任何运算都伴随
`Interface()` 装箱。长期方案：为脚本内数值引入未装箱表示
（如 `value.IntValue{int64}` 平铺结构），仅跨接口边界时回退 reflect。
改动面大（value 包 + 所有 run* 指令），仅在 P0/P1 后仍不达标时启动。

### P2-4 顺带修正：Interrupt 未真正生效

基准期间发现 `Executor.Interrupt` 只记日志、不取消 context，
超时定时器实际无法中断执行（executor.go:113-123）。
结合 P0-3 把 context 检查收敛到块边界后，让 Interrupt 调用 `cancelFunc`
即可真正生效。补一个超时中断端到端测试。

## 4. 执行顺序与验收门槛

| 阶段 | 内容 | 状态 | 门槛（对比 bench_report 基线） |
|---|---|---|---|
| 1 | P0-1 P0-2 P0-3 | ✅ 完成 | Fib18 allocs/op 降 ≥40%，ns/op 降 ≥30%；全套测试通过 |
| 2 | P1-1 P1-2 | ✅ 完成 | Fib18 累计 ≥5 倍提速；数值语义测试全绿 |
| 3 | P1-3 P1-4 | ✅ 完成 | ExecCall100 ≥2 倍；-race 全绿 |
| 4 | P2-1 P2-2 | ✅ 完成 | 分发开销占比 <2%（pprof，visitInstr 已退出 profile） |
| 5 | P2-4 | ✅ 完成 | 超时可真实中断，新增测试通过 |

每阶段结束更新 `gofunction/bench_report.md` 的「优化后」对比表。

## 5. 风险与守恒

- **map 缓存并发**：P0-1/P0-2 缓存写入只发生在 BuildProgram 后首次执行；
  多 goroutine 并发 Run 同一 Program 时需 `sync.Map` 或构建期预热，
  否则有数据竞争（-race 必须覆盖 `BenchmarkProgramRunParallel` 场景）。
- **frame 池与 go 语句**：goCall 对 fr.env 做快照拷贝，池化后归还的 frame
  不得残留被 goroutine 引用的 env；归还时机在 `waitGoroutines` 之后即可保证。
- **语义回归**：所有优化后跑全量 `go test -race ./...`，并保留
  `BenchmarkFib`（testdata_test）作为历史对照。
