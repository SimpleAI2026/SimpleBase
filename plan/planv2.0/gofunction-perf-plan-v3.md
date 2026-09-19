# gofunction 性能优化方案 v3（P3 阶段）

- 日期：2026-09-19
- 前置：`plan/planv2.0/gofunction-perf-plan.md`（P0/P1/P2 已完成）、`gofunction/bench_report.md`
- 当前水位：Fib18 慢于原生 Go **~340 倍**（2,662,758 ns vs 7,816 ns），58,546 allocs/op
- 本方案目标：再取 **2～3 倍**提速、分配再降 **60%+**，把差距压到 ~120 倍级别

> **进度（2026-09-19）**：全部四阶段（P3-1/2/3/4/5）已落地，
> `go build` / `go vet` / `go test -race ./...` 全绿。
>
> - 阶段 1+2（P3-2/P3-4/P3-5）：Fib18 分配 58,546 → 37,635（-36%）
> - **阶段 3（P3-1 env 槽位化）+ 阶段 4（P3-3）**：
>   - **ExecFib18：37,635 → 7 allocs（-99.98%），1.78ms（阶段累计 -28%，总 4.9x）**
>   - **ExecCall100：821 → 25 allocs（-97%），28.5μs（总 3.8x）**
>   - **ExecArithLoop：10,921 → 1,829 allocs（-83%），349μs（总 5.8x）**
>   - RunEndToEnd：307,668 → 234,956 ns（总 4.7x）
>   - fib(25) 端到端：259.8ms → **52.4ms（5.0x），分配 2,064,185 → 74（-99.99%）**
>
> 实施要点与实测教训：
> 1. **指针语义下沉 reflect 层**：Alloc/FreeVar 槽存 `*T` 地址值
>    （`reflect.New` 产物），`Elem().Set()` 共享写入，
>    `*value.Value` 间接层彻底消除；SSA 保证被捕获参数自动装箱，
>    未捕获参数按值入槽安全。
> 2. **P3-3 逃逸分析陷阱（重要）**：参数切片抽出辅助函数求值会触发
>    「返回值逃逸」判定，每次调用恒堆分配——fib18 从 11 allocs 暴涨到
>    8,371。参数切片必须**就地 make**（内联在 callOp 内），
>    逃逸分析才能将其栈分配。
> 3. **小整数缓存区间扩至 ±4096**：覆盖取模/位运算中间值，
>    ArithLoop 分配再降 37%。
>
> 剩余分配已触及 reflect 固有成本（`intValue` 区间外真装箱、
> `reflect.Append`），进一步优化需 P3-6 typed value（v4 立项）。

---

## 1. 剖析结论（P2 后重新采样）

### 1.1 分配热点（BenchmarkExecFib18，alloc_objects）

| 排名 | 位置 | 占比 | 根因 |
|---|---|---|---|
| 1 | `runBinOp` instruction.go:47 | **36.1%** | `fr.set` → `fr.env[instr] = &val`，每条指令一次 `*value.Value` 取址堆分配 |
| 2 | `convIntTyped` eval.go:436 | **21.3%** | 非预声明目标类型走 `reflect.Value.Convert` 分配 |
| 3 | `callOp` eval.go:495 | 14.4% | 每次调用 `make([]value.Value, n)` 参数切片 |
| 4 | `runCall` instruction.go:166 | 14.4% | 同 1（`fr.env[instr] = &v`） |
| 5 | `binopCompareFast` eval.go:414 | 13.7% | 比较结果 `reflect.ValueOf(bool)` + 接口装箱 |

跨场景验证（ArithLoop / Call100 / Map100 / Closure100 四个基准的 top6）：
`runBinOp`、`convIntTyped`、`runPhi`（instruction.go:243）、`binopCompareFast` 是**共性头部**，
合计占 70～92%。Closure100 额外有 `RValue.Elem`、`value.Package`、`makeFunc.func1` 各约 10%。

### 1.2 CPU 构成

- `runtime.kevent` 61% 为 profile 采样期的调度空转噪音，非真实开销。
- 有效热点：`runFrameCompiled` cum **28.5%**；其中
  - `mapaccess2` 4.3% + `mapassign` 2.8% + `typehash` 2.5% + `efaceeq` 1.2% + `memhash64` 0.9%
    ≈ **11.7% 花在 `map[ssa.Value]*value.Value` 的接口 key 哈希与比较上**
  - `mallocgc` 路径 8.6%
- `GOGC=off` 对照：2,662,758 → 2,598,231 ns（仅 **-2.4%**）。
  **结论：瓶颈不是 GC，而是分配路径本身的 CPU 成本 + map 查找开销。**
  这意味着「减少分配次数」的收益主要来自 malloc 快路径与 map 操作，而非 GC 压力缓解。

### 1.3 可行性微基准（已实测）

**env 存储结构**（64 槽位，读写各一次）：

| 方案 | ns/op | B/op | allocs/op |
|---|---|---|---|
| A. `map[ssa.Value]*Value` + 取址（现状） | 36.14 | 48 | 2 |
| B. `[]Value` 槽位切片 | 18.82 | 32 | 1 | **-48% 时间** |
| C. `[]reflect.Value` 槽位（去接口装箱） | **6.37** | **8** | **0** | **-82% 时间，零分配** |

**常用值单例化**：

| 方案 | ns/op | allocs/op |
|---|---|---|
| bool 每次构造 `RValue{reflect.ValueOf(b)}` | 13.53 | 1 |
| bool 预构造接口单例 | **1.59** | **0** |
| 小整数每次构造 | 17.93 | 1 |
| 小整数查表（-256..1024） | **1.68** | **0** |

**槽位化前提验证**（新写临时测试枚举 SSA 值，已确认）：
函数内所有可能成为 env key 的值（Params + FreeVars + Locals + 产生值的 Instruction）
在 SSA 构建后**完全静态可枚举**，可在预编译期分配连续槽位号：

```
add            params=2 freevars=0 locals=0 instrValues=1 total=3
fib            params=1 freevars=0 locals=0 instrValues=6 total=7
withClosure    params=1 freevars=0 locals=0 instrValues=7 total=8
withClosure$1  params=1 freevars=1 locals=0 instrValues=2 total=4
```

典型函数只有 3～8 个槽位 —— 用切片替换 map 不仅更快，内存也更省。

---

## 2. 优化项

### P3-1 env 槽位化（收益最大，改动最大）

**问题**：`frame.env` 是 `map[ssa.Value]*value.Value`。
接口类型 key 触发 `typehash`/`efaceeq`（~11.7% CPU）；
`fr.set` 的 `&val` 每次一次堆分配（**36% 分配对象数**）。

**方案**：预编译期为每个 `*ssa.Function` 建立 `map[ssa.Value]int` 槽位表（一次性，随
`compiledBlock` 缓存在 `Program`），运行期 `frame.env` 改为 `[]value.Value` 切片：

```go
// 预编译期（compile.go），每函数一次
type funcLayout struct {
    slots   map[ssa.Value]int // 仅编译期使用
    nSlots  int
    // 指令闭包直接捕获槽位号，运行期零查表
}

// 运行期
type frame struct {
    env []value.Value   // 按槽位号索引，替代 map
    ...
}
func (fr *frame) setSlot(i int, v value.Value) { fr.env[i] = v }  // 无取址、无分配
```

`compileInstr` 生成闭包时把 `instr` 的槽位号**编译期绑定进闭包**，
运行期连 map 查找都省掉，直接 `fr.env[slot] = v`。

**难点与对策**：
1. **`*ssa.Alloc` / `*ssa.FreeVar` 需要稳定地址**（`runStore`、`makeFunc` 捕获闭包变量
   依赖 `*value.Value` 指针语义）。
   → 保留一个 `cells []*value.Value` 辅助数组，**只有 Alloc/FreeVar/Local 占用 cell**
   （典型函数 0～2 个），其余指令值走无指针槽位。
2. **`goCall` 的 env 快照**：现为 map 拷贝，改为切片 `copy`（更快）。
3. **frame 池化**：`env` 切片按 `nSlots` 复用，`cap` 不足时重新分配，`clear` 保留容量。

**预期**：分配 -36%，CPU -12%（map 开销消失）+ malloc 快路径减少 → **整体 1.5～1.8x**。
**风险**：中高。涉及 `frame.get/set`、`runAlloc`、`runStore`、`makeFunc`、`callSSA`、`goCall`
六处核心逻辑。必须分两步提交（先建槽位表并双写校验，再切换读路径）。

---

### P3-2 常用值单例表（收益高，改动小，建议优先做）

**问题**：`binopCompareFast` 每次比较都 `reflect.ValueOf(bool)` + 接口装箱（13.7% 分配）；
小整数运算结果同理。

**方案**：包级预构造 `value.Value` 单例：

```go
// eval.go
var (
    trueValue  value.Value = value.RValue{Value: reflect.ValueOf(true)}
    falseValue value.Value = value.RValue{Value: reflect.ValueOf(false)}
    // 预声明 int 的小整数缓存，覆盖循环计数、索引等绝大多数场景
    smallInts [1281]value.Value // -256 .. 1024
)

func boolValue(b bool) value.Value {
    if b { return trueValue }
    return falseValue
}
```

`binopCompareFast` 返回 `boolValue(r)`；`convIntTyped` 在
`rt.Kind()==Int && rt.PkgPath()==""` 且值落在缓存区间时查表返回。

**注意**：单例安全性 —— `value.RValue` 底层 `reflect.Value` 不可寻址（`reflect.ValueOf`
产物），解释器不会对其调用 `Set`，无共享可变风险。需加单元测试固化该不变量。

**预期**：Fib18 分配 -14%（bool）+ 部分 int 命中；ArithLoop 命中率更高。**~1.15x**。
**风险**：低。局部改动，语义由现有 testdata 双执行兜底。

---

### P3-3 调用参数切片池化 / 小参数栈数组（收益中，改动小）

**问题**：`callOp` 每次调用 `make([]value.Value, len(instr.Args))`（14.4% 分配对象）。

**方案**：两选一或组合：
1. **小参数免堆**：`var buf [4]value.Value`，`len(args)<=4` 时用 `buf[:n]`
   —— 但 `args` 会被 `callSSA` 通过 `fr.env[p] = &args[i]` 取址逃逸，
   需先完成 P3-1（槽位化后参数改为值拷贝入槽，不再取址）才能生效。
2. **`sync.Pool` 分级池**：按 `len` 分 4/8/16 三档池化，`callSSA` 返回后归还。

**建议**：**依赖 P3-1 完成后做方案 1**（更彻底，零分配），否则做方案 2（约 -8% 分配）。
**预期**：**~1.1x**。**风险**：低（方案 2）/ 中（方案 1，须确保 args 不逃逸）。

---

### P3-4 convIntTyped 覆盖面扩展（收益中，改动极小）

**问题**：`convIntTyped` 现仅对「预声明 `int`」免转换，其余（`int64`、`int32`、
`uint`、命名类型）全部走 `reflect.Value.Convert`，占 21.3% 分配。

**方案**：按 Kind 分支直接构造对应宽度的具体类型值，绕过 `Convert`：

```go
switch rt.Kind() {
case reflect.Int:   if rt.PkgPath()=="" { return ...reflect.ValueOf(int(r)) }
case reflect.Int64: if rt.PkgPath()=="" { return ...reflect.ValueOf(r) }
case reflect.Int32: if rt.PkgPath()=="" { return ...reflect.ValueOf(int32(r)) }
...
}
// 命名类型仍走 Convert（P2 已验证：time.Weekday/Duration 必须保真）
```

**注意**：这是 P2 阶段踩过两次坑的地方 —— **必须保留 `PkgPath()==""` 守卫**，
否则 `time.Weekday`、`time.Duration` 等命名类型会退化成裸整型导致 switch 比较失配。
`reflect.ValueOf(int32(r))` 对小值仍可能分配（Go runtime 小整数有部分 staticuint64s
优化，仅覆盖 0-255 的单字节值），实际收益需实测确认。

**预期**：**~1.1x**，`uint`/`int64` 脚本收益更明显。**风险**：低（有 testdata 双执行守卫）。

---

### P3-5 Phi 节点预解析（收益中，改动小）

**问题**：`runPhi` 每次执行都遍历 `instr.Block().Preds` 线性查找 `fr.prevBlock`
（instruction.go:243，在 ArithLoop/Call100/Map100 中占 13～21% 分配 + 线性扫描开销）。

**方案**：预编译期把 `Preds` 索引固化 —— 为每个 Phi 生成
`map[*ssa.BasicBlock]ssa.Value`（或前驱数少时用小数组 + 指针比较），
闭包直接捕获，运行期 O(1) 取边值。进一步地，若该 Phi 的所有边值同为常量，
可在编译期直接折叠。

**预期**：**~1.05～1.1x**（循环密集脚本更高）。**风险**：低。

---

### P3-6（研究项，不在本期）typed value 体系

彻底去掉 `value.Value` 接口与 `reflect.Value` 包装，改为 tagged union：

```go
type Val struct {
    kind uint8
    i    int64      // int/uint/bool/float 共用
    ref  any        // 仅复合类型使用
}
```

微基准显示这是唯一能把 env 读写压到 **6.37 ns / 0 allocs** 的路径（方案 C），
理论上可再取 2～3 倍。但需重写 `value` 包全部实现与所有指令，
且与外部反射调用（`callExternal`）的边界转换需要仔细设计。

**建议**：作为 v4 独立立项，先完成 P3-1~P3-5 取得确定收益。

---

## 3. 执行顺序与验收门槛

| 阶段 | 内容 | 状态 | 实测 | 风险 | 门槛 |
|---|---|---|---|---|---|
| 1 | **P3-2**（值单例表） | ✅ 完成 | Fib18 allocs -36%（含阶段 2） | 低 | Fib18 allocs -12%；testdata 全绿 ✅ |
| 2 | **P3-4**（conv 覆盖面）+ **P3-5**（Phi 预解析） | ✅ 完成 | ArithLoop allocs -49% | 低 | ArithLoop allocs -20%；命名类型回归测试全绿 ✅ |
| 3 | **P3-1**（env 槽位化） | ✅ 完成 | Fib18 allocs **-99.98%**（7/op） | 中高 | Fib18 allocs -50% ✅ 超额达成；`-race` 全绿 ✅；闭包/defer/recover 全部现有测试通过 ✅ |
| 4 | **P3-3**（参数栈数组，依赖 P3-1） | ✅ 完成 | Call100 allocs **-97%**（25/op） | 中 | Call100 allocs -30% ✅ 超额达成 |

### 阶段 1+2 实测结果（2026-09-19）

| 基准 | P2 后 allocs | P3 阶段 1+2 后 | 降幅 | ns/op 变化 |
|---|---|---|---|---|
| ExecArithLoop | 16,614 | **10,921** | -34% | 562,286 → 474,692（-16%） |
| ExecFib18 | 58,546 | **37,635** | -36% | 2,724,162 → 2,317,890（-15%） |
| ExecCall100 | 1,089 | **821** | -25% | 44,987 → 43,645（-3%） |
| ExecMap100 | 1,944 | **1,389** | -29% | 77,018 → 69,545（-10%） |
| ExecMethod100 | 2,037 | **1,736** | -15% | 66,345 → 59,258（-11%） |
| RunEndToEnd | 10,344 | **7,531** | -27% | 351,896 → 307,668（-13%） |

新增回归测试（`valuecache_test.go`）：
- `TestValueCacheImmutable` — 固化单例不可寻址、不可 Set 的安全前提
- `TestBoolValueSemantics` / `TestIntValueBoundary` — 单例与直接构造等价性、缓存边界内外一致
- `TestConvTypedWidthFidelity` — 各宽度截断行为与 Go 原生转换一致
- `TestConvTypedNamedTypeFidelity` — **命名类型不被降级**（P2 踩坑两次的守卫）
- `TestPhiMultiPred` / `TestPhiLoopCarried` — Phi 预解析在 switch 多前驱与循环回边下的正确性

### 阶段 1+2 后剖析（剩余分配已高度集中）

| 来源 | 占比 | 对应方案 |
|---|---|---|
| `runBinOp` instruction.go:47（`fr.env[instr] = &val`） | **55.8%** | P3-1 |
| `runCall` instruction.go:166（同上） | 24.1% | P3-1 |
| `callOp` eval.go:526（参数切片 make） | 19.9% | P3-3 |

合计 **99.8%**。`convIntTyped` 与 `binopCompareFast` 已完全退出热点列表。

每阶段必过门槛：
- `go build ./... && go vet ./...`
- `go test -race -count=1 ./...`（含 testdata 解释/原生双执行比对）
- `go test -run='^$' -bench=. -benchmem -count=3 .` 无回退项

---

## 4. 结论：还能优化吗？

**能，且空间明确。**

- **短期（P3-2/4/5，低风险）**：约 **1.3 倍**，改动集中在 3 个函数，1～2 小时工作量。
- **中期（P3-1/3，中高风险）**：累计 **2.2～2.7 倍**，把 env 从 map 降级为切片是
  当前性价比最高的结构性改动，微基准已证实上限。
- **长期（P3-6）**：typed value 可再取 2～3 倍，但等价于重写解释器值层，
  应独立立项评估。

需要注意的**天花板**：只要值表示仍是 `reflect.Value`，每个中间值就至少有一次
`reflect.ValueOf` 的隐式装箱成本（微基准 5.6 ns/次）。P3-1~P3-5 是在这个天花板下
榨取剩余空间；要突破需要 P3-6。
