# gofunction 优化计划（v6.5）

> **状态（2026-11-03）：全部批次完成**（B1 → A1 → A2 → A3 → B2 → B3 → C1–C5 → D1–D2）。
> 结果摘要：ExecFib18 1.93ms→**1.04ms**（1.85x）、ExecArithLoop 385μs→**163μs**（2.37x）、
> RunEndToEnd 250μs→**156μs**（1.60x）；RunJSON 缓存命中 19μs vs 未命中 59μs（3.0x）。
> 附带修复两个既有 bug（全局闭包绑定已取消 context；funcCache 跨 Run 复用）。
> 偏差记录：A4 未执行（skipFastEval 测试 seam 依赖 binop 数值分支，删除破坏测试连通
> 且无运行期收益，决策保留）；验收线 fib18 ≤0.9ms 达成 1.04ms（差 15%，剩余为
> callSSA/reflect.MakeFunc 固有成本）。详见 `gofunction/bench_report.md` v6.5 章节。

> 范围：`gofunction/` 独立模块（~13.8k 行，其中 `internal/interp` 为解释器核心），以及宿主 `internal/api/gofunction_handler.go`、`internal/functionrun/runner.go`、`internal/app/cron_runner.go` 三个 RunJSON 调用点。
> 基线：`bench_report.md`（2026-09-19，P0–P3 完成后）：ExecFib18 1.78ms（原生 ~327 倍）、ExecArithLoop 349μs、RunEndToEnd 235μs；BuildProgram 23μs/610 allocs（无 import 场景）。
> 约束：不改公共 API 门面（`gofunction.go` 类型别名契约由 `facade_test.go` 锁定）；`value.Value` 接口保持（P3-6 tagged union 明确不做等价重写）；不新增依赖。

---

## 一、现状与核心发现

P0–P3 已做完：类型映射缓存、常量缓存、frame 池化、makeFunc 缓存、分发表编译期化（闭包切片）、Phi 边预解析、env 槽位化（`[]value.Value`）、参数零堆化、值单例表。

本次分析发现的**未兑现设计**与**新空间**：

### 发现 1（最重要）：槽位号没有编译进指令闭包

`frame.go:get()` 注释声称「预编译闭包内已把热路径槽位号直接编译进指令闭包」，但 `compile.go:compileInstr` 的实现是：

```go
case *ssa.BinOp:
    return func(fr *frame) step { return runBinOp(fr, instr) }  // 只捕获 instr
```

而 `runBinOp → binop(instr, fr.get(instr.X), fr.get(instr.Y)) → fr.set(instr, v)`：**每次执行**都要走 `slotOf` 的 `map[ssa.Value]int` 查找（接口键哈希 + efaceeq），一条 BinOp 3 次（X/Y/结果），Store/Call 同理。P3-1 消除的是 env 的 map，**取址这一层 map 还在热路径上**。对照：`compilePhi` 已正确固化 preds/edges——同一手法未推广到其余指令。

### 发现 2：运算类型与算符未做编译期特化

`binopFast` 每次执行跑 `x.Kind()` switch × `instr.Op` switch（两次分发）。SSA 下这两个值编译期恒定，可按 (op, 类型) 生成特化闭包（部分求值），执行期零分发直达 `convIntTyped(a+b, rt)`。

### 发现 3：RunJSON 每请求全量重建 Program

宿主 4 个调用点（HTTP 同步 ×2、cron、functionrun）全部走 `RunJSON`，每次 parse + typecheck + SSA 构建 + init 执行 + `compileProgram` 预热。同一函数被反复调用时（生产常态：一个云函数版本被 N 次请求），全部重复。**但直接缓存 Program 有两个语义/安全障碍**：

- `Program.globals` 是普通 map，`runStore` 的 `*ssa.Global` 分支直接 `fr.program.globals[addr] = &val`——并发复用会数据竞争；
- 包级 `init` 与全局变量在每次 BuildProgram 时重置，缓存复用会**残留上次执行的副作用**（语义变化）。

### 发现 4：常量操作数每次走 sync.Map

`fr.get(*ssa.Const)` → `constValueCached` → `sync.Map.Load`。SSA 常量不可变，基础类型常量可在编译期直接烤进闭包，运行期零查表。

### 发现 5：代码债

- `binop` 慢路径（eval.go ~137–280 行）与 `binopFast` 数值分支**整段重复**，快路径失败后本应只剩 string/complex/比较；
- `Executor.waitEventLoop` 写不读；`Compile`/`SetProfile`/`GetProfile` 空桩且宿主零调用；`Runtime.userData`（GetUserData/SetUserData）全仓无引用；
- `runFrame`（非编译主循环）生产不可达（`callSSA` 只调 `runFrameCompiled`），仅测试与 `compileInstr` default 分支的 `visitInstr` 兜底在使用；
- `conv`（type.go）与 `convTyped`/`convIntTyped`（eval.go）命名与职责重叠；
- `Executor.vm *Runtime` 包装过度，实际只用 SequenceID/Program 两字段。

---

## 二、优化项清单

### 批次 A：解释器核心提速（PF-1 / PF-2 / PF-4 + 收敛 PC-2）

| 项 | 内容 | 预期 |
|---|---|---|
| **A1 槽位固化** | `compileInstr` 在编译期用 `layout.slotOf` 解析操作数/结果槽位号，把 **int 直接捕获进闭包**：`return func(fr) step { v := binop(instr, fr.env[sx], fr.env[sy]); fr.env[sr] = v; return stepNext }`。适用于 BinOp/UnOp/Store/Load 类（UnOp/Field/Index/Extract/Return/MakeInterface/MapUpdate/Send…全部两三操作数指令）。**边界**：操作数不是本函数 SSA 值（Const/Global/Function/外部 FreeVar）时保留 `fr.get` 慢分支——编译期判别，不在运行期判断。 | 消除热路径全部 map 查找；算术/递归类预估再 1.5–2.5x |
| **A2 运算特化** | BinOp/UnOp 编译期按 (`instr.Op`, `instr.X.Type()` 底层类型) 生成特化闭包，如 int 加法直达 `convIntTyped(a+b, rt)`；无法特化（string/complex/命名类型）回落现有 `binopFast`→`binop` 链。命名类型守卫沿用 `convIntTyped` 的 `PkgPath() == ""` 规则（P2 踩坑记录）。 | 消除双重 switch 分发，叠加 A1 预估累计 2–3x |
| **A3 常量烤入** | 编译期对 `*ssa.Const` 操作数直接 `constValue` 求值并捕获进闭包（基础类型单例）；复合零值保持运行期新建（现有语义）。 | 消除热路径 sync.Map.Load |
| **A4 收敛 binop 慢路径** | `binop` 删除与 `binopFast` 重复的数值分支，只保留 string/complex/比较与 panic 兜底；`skipFastEval` 测试 seam 语义不变（走原路径的测试改指向保留分支）。 | -140 行重复代码，行为等价 |

**A 批风险**：闭包捕获数量增加 → 编译期内存与 `compileProgram` 预热耗时上升（一次性）；固化的前提是布局不可变（`layoutCache` 已保证）。每步用 `go test -race ./...` + 全套 bench 对照，任何基准回退 >5% 即回滚该项。

### 批次 B：宿主端到端（PC-7 前置 + PF-3 缓存 + PF-7）

| 项 | 内容 | 预期 |
|---|---|---|
| **B1 globals 并发安全** | `runStore` 的 Global 分支与 `initGlobal` 改为 RWMutex 保护（或 per-run overlay map，读时先查 overlay 再查共享表）。**这是缓存的前置修复**，独立合并。 | 修复潜在竞态 |
| **B2 Program 编译缓存（两阶段）** | `BuildProgram` 拆分：`compileProgramOnly`（parse/typecheck/SSA/blockCache/layoutCache/funcCache 预热——**不可变、可共享**）+ `initState`（initGlobal + 重跑包级 init + globals 重置——**每请求执行**）。`RunJSON` 增加内部 LRU（key = sha256(source)+funcName，容量默认 128，`sync.Map` + 双向链表或简单计数淘汰），命中时浅拷贝 Program 壳 + 重建 globals + 重放 init。语义与现状完全一致（init 每请求执行），只是跳过重编译。**缓存只挂在 RunJSON 内部**，`BuildProgram` 公共 API 行为不变。 | 同脚本重复调用省 parse+typecheck+SSA（实测 23μs 起，带 import 的真实脚本预计 1–10ms）；热函数端到端预估 2–50x |
| **B3 callExternal 免 Convert** | 参数类型 `AssignableTo(fnType.In(i))` 时跳过 `.Convert`（Convert 对已同型值有分配）。热点：`fmt`/`strings`/`json` 外部调用。 | 外部调用路径 5–15% |

**B 批风险**：B2 必须先验证「init 重放」覆盖全部全局副作用路径（`initGlobal`/脚本 init/依赖包 init 三处）；缓存条目随源码版本失效由宿主天然保证（每次保存生成新 source）。LRU 内存上限：Program 含 SSA 常驻，128 条 × ~1MB 级，可配置。

### 批次 C：清理（PC-1 / PC-3 / PC-4 / PC-5 / PC-6）

| 项 | 内容 |
|---|---|
| C1 | A1 落地后修正 `frame.go:get()` 注释为真实描述；若 A1 不做某指令，注释明确列出 |
| C2 | 删除 `Executor.waitEventLoop`、`Compile`、`SetProfile`、`GetProfile`、`Runtime.userData`/`GetUserData`/`SetUserData`（全仓 grep 确认零调用后删；`facade_test.go` 若有转发断言同步更新） |
| C3 | `runFrame`/`visitInstr` 收敛：`visitInstr` 保留（unknown 指令兜底 + 测试 seam），`runFrame` 仅测试引用 → 移到 `_test.go` 或标注 `// test-only`；生产路径唯一入口 `runFrameCompiled` |
| C4 | `conv` 更名 `convSlow`（或并入 `constValue` 私有），消除与 `convTyped` 家族的命名歧义；全仓引用同步 |
| C5 | `Executor.vm` 简化：`Runtime` 只保留 `SequenceID`/`Program` 字段（userData 删除后其余字段无用），或直接内联进 Executor——以最小 diff 为准 |

### 批次 D：基准与文档

- D1：全套 bench 重跑（`internal/interp/bench_test.go` + `bench/`），`bench_report.md` 增补 v6.5 章节：A 批逐项对照（fib18/arith/call/slice/map）、B 批端到端（RunJSON 首次 vs 缓存命中）。
- D2：README「工作原理」补两阶段编译与缓存说明。

---

## 三、实施顺序与验证

**顺序**：B1（独立修竞态，先行合并）→ A1 → A2 → A3 → A4 → B2 → B3 → C 批 → D 批。
理由：A 批改变闭包形态，先做避免 B2 缓存预热路径返工；B1 独立且是 B2 前置；C 批纯清理放最后。

**每批验证**：

```bash
cd gofunction
go build ./... && go vet ./...
go test -race -count=1 ./...                     # 四包全绿
go test -run='^$' -bench=. -benchmem -count=3 ./internal/interp/
go test -run='^$' -bench=. -benchmem ./bench/    # 端到端对照
cd .. && go build ./... && go test ./internal/api -run 'TestGofunction|TestFunction' -count=1
```

**验收线**：ExecFib18 ≤ 0.9ms（当前 1.78ms，A1+A2 目标 ~2x）；RunJSON 缓存命中路径 ≤ 60μs（当前 235μs 端到端，B2 后由执行净耗时主导）；全量 `-race` 无新增告警；bench 无任何项回退 >5%。

**明确不做**：P3-6 tagged union（等价重写值层，收益/风险比差）；JIT/机器码生成（超出 reflect 解释器范畴）；`encoding/json` → json-iterator 的宿主侧替换（RunJSON 编码占比小，且 json-iterator 已作为脚本可选包注册，避免语义漂移）。
