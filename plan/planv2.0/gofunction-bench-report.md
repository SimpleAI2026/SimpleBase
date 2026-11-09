# gofunction 性能测试报告（P0~P3 全量优化后）

- 日期：2026-09-19
- 环境：macOS / Apple M4 Pro（arm64，14 核），go 1.25.0（实测 1.26.1 runtime），golang.org/x/tools v0.41.0
- 基准代码：`gofunction/bench_test.go`
- 优化方案：`plan/planv2.0/gofunction-perf-plan.md`（P0/P1/P2）、
  `plan/planv2.0/gofunction-perf-plan-v3.md`（P3-1/2/3/4/5 全部完成）
- 验证：`go build` / `go vet` / `go test -race -count=1 ./...` 四包全绿

## 1. 优化内容总览

| 项 | 实现 | 关键文件 |
|---|---|---|
| P0-1 类型映射缓存 | `types.Type` 直接索引替代 `TypeString` | type.go, importer |
| P0-2 常量求值缓存 | `Program.constCache` | program.go |
| P0-3 ctx 检查降频 | 每块一次 `context.Err()` | compile.go |
| P1-1/2 binop 免装箱 | `binopFast` 系列按目标类型直接产出 | eval.go |
| P1-3 frame 池化 | `sync.Pool` 复用 frame | call.go |
| P1-4 makeFunc 缓存 | `Program.funcCache` | frame.go |
| P2-1 分发表编译期化 | 基本块 → `[]instrStep` 闭包切片 | compile.go |
| P2-2 If/Jump 直通 | 块尾终结指令融合切换 | compile.go |
| P2-4 Interrupt 生效 | `cancelFunc` 注册 + context 取消 | executor.go |
| P3-2 值单例表 | bool/小整数（±4096）预构造 | valuecache.go |
| P3-4 conv 全宽度 | int8~64/uint8~ptr/float32/64 快路径 | eval.go |
| P3-5 Phi 预解析 | 前驱→边值编译期固化 | compile.go |
| **P3-1 env 槽位化** | **`map[ssa.Value]*value.Value` → `[]value.Value` 槽位数组**：编译期 `funcLayout` 为函数内全部 SSA 值分配连续槽位；运行期读写一次切片索引。**指针语义下沉 reflect 层**——Alloc/FreeVar 槽存 `*T` 地址值，`Elem().Set()` 共享写入，`*value.Value` 间接层彻底消除 | layout.go, frame.go, call.go |
| **P3-3 参数零堆化** | `callOp` 参数切片就地 make（逃逸分析栈分配）；实测教训：**抽出辅助函数会触发「返回值逃逸」判定恒堆分配**（fib18 从 11 → 8371 allocs），必须保持内联 | eval.go |

### 槽位化设计要点（P3-1）

- **编译期**：`buildLayout` 按 Params/FreeVars/Locals/产值指令顺序分配槽位号，
  随 `Program.layoutCache` 缓存，并发 Run 只读共享。
- **参数按值入槽**：SSA 保证被闭包捕获的参数自动装箱为堆 Alloc
  （实测验证：`outer` 中 `n` → `Alloc t0 (*int)` + `Store`），未捕获参数只读，
  值拷贝安全。
- **地址共享**：`zero(deref(T))` 产出 `reflect.New` 地址值存槽，
  `runStore` 直接 `ptr.Elem().Set(src)`（ assignable 时免 Convert）。
- **goroutine 快照**：`goCall` 的 env 快照从 map 拷贝降为切片 `copy`。
- **池化适配**：`prepareEnv` 按 `nSlots` 复用容量，`clear` 释放引用。

## 2. 优化前后基准对比

### 2.1 解释执行

| 基准 | 基线 ns/op | P3 后 ns/op | 累计提速 | 基线 allocs | P3 后 allocs | 累计降幅 |
|---|---|---|---|---|---|---|
| ExecArithLoop(1000) | 2,015,528 | **349,171** | **5.8x** | 79,639 | **1,829** | **-98%** |
| ExecFib18 | 8,754,619 | **1,779,489** | **4.9x** | 317,717 | **7** | **-100%** |
| ExecCall100 | 108,272 | **28,546** | **3.8x** | 3,811 | **25** | **-99%** |
| ExecSlice100 | 273,947 | **102,329** | 2.7x | 10,053 | **2,038** | -80% |
| ExecMap100 | 215,758 | **53,742** | **4.0x** | 7,481 | **330** | -96% |
| ExecString100 | 122,611 | **37,370** | **3.3x** | 4,431 | **705** | -84% |
| ExecExternal100 | 93,776 | **38,305** | 2.4x | 3,235 | **608** | -81% |
| ExecClosure100 | 143,177 | **55,331** | 2.6x | 5,035 | **848** | -83% |
| ExecMethod100 | 145,027 | **45,653** | **3.2x** | 5,452 | **629** | -88% |

### 2.2 端到端与并发

| 基准 | 基线 | P3 后 | 累计提速 |
|---|---|---|---|
| RunEndToEnd | 1,101,303 | **234,956** | **4.7x** |
| ExecutorExecute | 1,108,843 | **234,677** | **4.7x** |
| PoolParallelExecute | 675,883 | **392,268** | 1.7x |
| ProgramRunParallel | 768,228 | **404,068** | 1.9x |

并发基准的分配已降到 7~2,457 allocs，进一步提速受 CPU 饱和限制而非分配器竞争。

### 2.3 与原生 Go 差距（Fib18 同轮次）

- 解释执行 1,779,489 ns vs 原生 5,435 ns → **~327 倍**（基线 ~1,576 倍）
- `BenchmarkFib`（fib(25) 端到端）：259.8ms → **52.4ms**（**5.0x**），
  分配 2,064,185 → **74**（**-99.99%**）

### 2.4 编译阶段

BuildProgram 23,012 ns / 610 allocs（含槽位布局构建，较基线 +5%，一次性成本）。

## 3. P3 后剖析

- **CPU**：map 相关开销（typehash/efaceeq/mapassign ~11.7%）全部消失；
  `visitInstr` 保持退出 profile。
- **alloc_objects**（ExecArithLoop）：剩余分配 98% 来自 `intValue` 的
  区间外真装箱（`sum` 累加值超 ±4096），属合理语义成本；
  ExecFib18 已达 **7 allocs/op**（全部为 frame 池冷启动）。
- 进一步优化空间：
  1. `value.Value` 接口 → tagged union（P3-6/v4 立项）：微基准上限
     再 2~3 倍，但等价重写值层；
  2. Slice/String 场景的 `reflect.Append`/`unsafe_New` 为 reflect 固有成本，
     需内置操作特化（如 `append` 快路径）才有收益。

## 4. 复现

```bash
cd gofunction
go test -race ./...                                # 全部通过
go test -run='^$' -bench=. -benchmem -count=3 .   # 本报告数据
go test -run='^$' -bench=ExecFib18 -memprofile=m.prof .
go tool pprof -top -lines -sample_index=alloc_objects m.prof
```
