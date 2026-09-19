# gofunction 简化重构计划（Naming & Structure Simplification Plan）

> **日期**：2026-09-18
> **前置**：本计划基于 `gofunction-refactor-plan.md`（已完成：并入主 module、重建 value 包、删除死代码、修复编译级 bug）。本阶段聚焦**命名、文件组织、功能合并**三项简化，不引入新功能。
> **范围**：`gofunction/` 约 10,700 行（含测试），核心代码约 3,400 行。

---

## 1. 现状问题清单（已核实）

### 1.1 文件名与内容错位

| 文件 | 问题 |
|---|---|
| `gofun.go` | 包名已是 `gofunction`，文件名仍是旧包名 `gofun`；内容仅 25 行（`newSequenceID` + 包文档） |
| `interpreter.go` | 内容是 `Program` 类型 + 构建/执行 API，与「解释器」语义错位；真正逐指令执行的 `visitInstr` 却在 `call.go` |
| `call.go` | 大杂烩：函数调用（callSSA/callBuiltin/callExternal）+ 帧循环（runFrame/visitInstr）+ 类型工具（zero/deref）+ 枚举（nextInstr），354 行 4 种职责 |
| `packages/packages.go` | 319 行清单式单文件，13 个标准库注册挤在一起 |

### 1.2 类型与命名混乱

1. **向后兼容别名堆砌**（无一有引用方，全部为死代码）：
   - `type ExternalObject = Object`、`type ExternalPackage = Package`（importer）
   - `type KeywordInfo = CompletionItem`、`func Keywords()`（Deprecated）（importer）
   - `type BasicKind = ObjectKind`（importer）
2. **枚举命名违背 Go 惯例**：`_NEXT`、`_Return`、`_JUMP` 下划线/大小写混用。
3. **过度设计**：`operations` 包 5 个空 struct（`UnaryOperations`/`BinaryOperations`/`ComparisonOperations`/`CallOperations`/`ConstantOperations`）+ `OperationManager` 纯转发层；顶层 `ops.go` 再包一层 `frameAdapter` 适配器——同一运算逻辑隔着「适配器 → manager → 空 struct」三层间接。
4. **半成品类型**：`RuntimeAsync`（waitGroup 从未接线）、`RuntimeTaskInfo`（无引用）、`Runtime.ConsoleLog`（内部实现用日志替代）、`ExecutorPool.registryPool`/`vmPool`（定义未用）、`CodeEntry`（无引用）。
5. **调试遗留**：`debug_temp_test.go`（临时调试文件）。

### 1.3 测试文件命名无规则

`gofun_test.go`、`interp_extra_test.go`、`interp_more_test.go`、`coverage_more_test.go`、`runframe_jump_test.go`、`testdata_scripts_test.go`、`value/value_more_test.go`——`extra/more/coverage` 后缀无信息量，与被测文件无对应关系。

---

## 2. 命名规范（统一定义，后续所有代码遵循）

### 2.1 通用规则

| 项 | 规则 | 示例 |
|---|---|---|
| 文件名 | 小写+下划线，与**内容职责**一致；测试文件 `<职责>_test.go` 与被测文件同名 | `program.go` / `program_test.go` |
| 导出类型 | 大驼峰；构造函数 `NewXxx` | `Program` / `NewImporter` |
| 私有类型 | 小驼峰，无下划线前缀 | `frame`、`step` |
| 枚举 | 类型 + 同前缀驼峰常量，禁用下划线开头 | `stepNext`、`stepReturn`、`stepJump` |
| 指令处理器 | 统一签名模板：`func runXxx(fr *frame, instr *ssa.Xxx) step` | `runPhi`、`runSelect` |
| 注册函数（packages） | 统一三个 helper 模板：`funcObj` / `constObj` / `varObj`，每标准库一个 `stdXxx()` 构造函数 | `stdFmt()`、`stdMath()` |
| 注释 | 文件头一行职责说明；导出符号中文 doc comment；内部逻辑英文关键词混排允许 | 现有风格保持 |

### 2.2 错误边界模板

- 解释器**内部**（指令执行层）：允许 panic（SSA 语义：Go 程序自身 panic）。
- **公共 API 边界**（`Run`/`RunWithContext`/`BuildProgram`）：recover 并返回 `error`，绝不向调用方泄漏 panic（现有行为保持，写入模板防止回归）。

---

## 3. 目标结构

```
gofunction/
├── gofunction.go        # 包文档 + 公共入口（Run/ParseFuncList/SetLogger/SetDebugOutput/newSequenceID）
├── program.go           # Program：BuildProgram / Run / RunWithContext / 全局变量 / external 解析
├── frame.go             # frame 栈帧 + Context 执行上下文
├── runframe.go          # runFrame 主循环 + visitInstr 分发 + step 枚举
├── instruction.go       # runXxx 指令处理器全集（单一职责，已是模板化，保持）
├── call.go              # callSSA / callBuiltin / callExternal / zero / deref（纯「调用」职责）
├── eval.go              # 表达式求值：unop / binop / constValue / goCall（原 operations 实现并入，无 manager 无适配器）
├── type.go              # typeChange / conv / builtinTypes
├── runtime.go           # Runtime（收敛为脚本运行时数据袋，删 Async/TaskInfo/ConsoleLog）
├── executor.go          # Executor + ExecutorPool + ExecutorPoolConfig（两文件合一）
├── value/               # 值抽象
│   ├── value.go         # Value 接口 + RValue + ValueOf/NewRValueOf + DefaultConverter（converter 并入）
│   ├── external.go      # ExternalValue + ExternalValueWrap
│   └── tuple.go         # Package/Unpackage + MapIter（package.go 并入）
├── importer/            # 包导入与注册表
│   ├── importer.go      # Importer（parser.go 的 parseNameType 并入，38 行单点使用）
│   ├── registry.go      # Registry / Object / Package
│   ├── types.go         # GetExternalType 等
│   └── completion.go    # 【决策点】代码补全——建议删除（与解释执行无关，无引用方）
├── packages/            # 标准库注册（清单拆分，模板化）
│   ├── packages.go      # init + mustRegister/funcObj/constObj/varObj 模板
│   ├── std_fmt.go       # stdFmt()
│   ├── std_strings.go   # ...
│   ├── std_math.go
│   ├── std_time.go
│   ├── std_regexp.go
│   ├── std_encoding.go  # json + base64
│   ├── std_misc.go      # errors / strconv / bytes
│   └── std_http.go
└── *_test.go            # 与被测文件一一对应（见 §5）
```

### 删除清单

| 项 | 理由 |
|---|---|
| `debug_temp_test.go` | 调试遗留 |
| `operations/` 整包 | 空struct+manager+适配器三层间接，实现并入 `eval.go`；测试迁移 |
| `value/converter.go`、`value/package.go` | 并入 `value.go`、`tuple.go` 后删除原文件 |
| `importer/parser.go` | 38 行并入 `importer.go` |
| `importer/completion.go`（+测试） | 【决策点】编辑器补全功能，解释执行无引用 |
| 别名 `ExternalObject`/`ExternalPackage`/`KeywordInfo`/`BasicKind`、`Keywords()` | 死代码 |
| `RuntimeAsync`/`RuntimeTaskInfo`/`Runtime.ConsoleLog` | 半成品/无引用 |
| `ExecutorPool.registryPool`/`vmPool` 字段、`CodeEntry` 类型 | 定义未用 |

---

## 4. 文件级映射表

| 现文件 | 去向 |
|---|---|
| `gofun.go` | → `gofunction.go`（+ SetLogger/SetDebugOutput 从 frame.go 移入 + ParseFuncList 从 interpreter.go 移入） |
| `interpreter.go` | → `program.go`（更名；去掉 ParseFuncList） |
| `call.go` | → `call.go`（调用部分）+ `runframe.go`（runFrame/visitInstr/step 枚举） |
| `ops.go` | 删除；适配逻辑消失，`unop`/`binop`/`constValue`/`goCall`/`callOp` 在 `eval.go` 直接实现 |
| `operations/{manager,binop,comparison,unary,external,call}.go` | 实现函数 → `eval.go` 与 `call.go`（外部调用）；空struct/manager/FrameInterface/frameAdapter 全删 |
| `executor.go` + `executor_pool.go` | → `executor.go`（合并） |
| `runtime.go` | 收敛（删 Async/TaskInfo/ConsoleLog） |
| `instruction.go`、`frame.go`、`type.go`、`value/value.go`、`value/external.go`、`importer/importer.go`、`importer/registry.go`、`importer/types.go` | 保留（frame.go 的 logger 移出） |
| `value/tuple.go` + `value/package.go` | → `tuple.go` |
| `value/converter.go` | → 并入 `value.go` |
| `packages/packages.go` | → 拆为 `packages.go` + 8 个 `std_*.go` |

### 4.1 枚举重命名

```
nextInstr → step
_NEXT     → stepNext
_Return   → stepReturn
_JUMP     → stepJump
```

---

## 5. 测试归并策略

| 现测试文件 | 去向 |
|---|---|
| `gofun_test.go`（TestAll 对拍/TestImportGofun/BenchmarkFib） | → `testdata_test.go`（对拍）+ `program_test.go`（ImportGofun/Bench） |
| `runframe_jump_test.go` | → `runframe_test.go` |
| `interp_extra_test.go`、`interp_more_test.go`、`coverage_more_test.go`、`testdata_scripts_test.go` | 逐个甄别：与源码文件对应归并到 `program_test.go`/`call_test.go`/`eval_test.go`/`instruction_test.go`，重复用例去重 |
| `executor_test.go`、`runtime_test.go` | 保留（对应新文件） |
| `operations/*_test.go`（6 个，约 1,900 行） | 运算用例 → `eval_test.go`；外部调用用例 → `call_test.go`；纯 manager/适配器测试删除 |
| `value/value_more_test.go`（+散落测试） | → `value/value_test.go` |
| `importer/*_test.go` | 保留；completion 相关随决策点删除 |
| `packages/packages_test.go` | → `packages/std_test.go` |

原则：**每个源文件至多一个同名测试文件**；`xxx_more_test.go`/`xxx_extra_test.go` 后缀消失。

---

## 6. 实施步骤（每步可独立验证）

1. **Step 1 清死**：删 `debug_temp_test.go`、全部向后兼容别名、`RuntimeAsync`/`RuntimeTaskInfo`/`ConsoleLog`、未用字段。验证：`go build` + `go test` 不回归。
2. **Step 2 顶层重组**：文件改名/拆分/合并（§4 映射）；枚举重命名；`gofun.go` → `gofunction.go`。纯移动+改名，不改逻辑。验证：build/test 全绿。
3. **Step 3 消灭 operations 间接层**：实现函数并入 `eval.go`/`call.go`，删除空struct/manager/FrameInterface/frameAdapter；迁移测试。验证：原 operations 测试用例在新位置全部通过。
4. **Step 4 子包整理**：value 三文件归并；importer 删 parser.go（并入）、决策 completion.go；packages 拆 `std_*.go` 并统一注册模板。
5. **Step 5 测试归并**：按 §5 重命名归并，去重。
6. **Step 6 验收**：`go build ./gofunction/...`、`go test ./gofunction/... -count=1`、`go vet ./gofunction/...`、主工程 `go build ./...`；TestAll 对拍通过率不低于重构前。

## 7. 风险

| 风险 | 对策 |
|---|---|
| operations 测试迁移丢失用例 | 迁移前记录用例数，迁移后比对计数；body 语义不变仅改 import |
| 文件移动引入 import 循环（eval.go 需同时依赖 value 与顶层 frame） | operations 本就依赖 value；顶层 eval.go 依赖 value 无循环风险 |
| completion.go 删除影响潜在功能 | 列为决策点：默认删除，git 历史可回溯；如主工程后续需要编辑器补全再恢复 |
| 「先不写代码」约束下 plan 与现状漂移 | 本 plan 基于当前实际符号清单（§1 已核实），映射表逐文件列出，执行时直接对照 |

## 8. 验收标准

- [ ] 文件名与包名一致：无 `gofun.go`；`interpreter.go` 更名 `program.go`
- [ ] 无 `xxx_more_test.go`/`xxx_extra_test.go` 类测试命名；每个源文件至多一个同名测试文件
- [ ] 无向后兼容别名（ExternalObject/ExternalPackage/KeywordInfo/BasicKind）
- [ ] `operations/` 目录消失；运算逻辑在顶层 `eval.go`，无 manager/空struct/适配器
- [ ] 枚举符合 Go 惯例：`step{Next,Return,Jump}`
- [ ] `Runtime` 无未接线字段；`Executor` 相关合并为单文件
- [ ] packages 按 `std_*.go` 拆分且使用统一注册模板
- [ ] `go build` / `go vet` / `go test` 全绿；TestAll 对拍通过数不低于重构前
