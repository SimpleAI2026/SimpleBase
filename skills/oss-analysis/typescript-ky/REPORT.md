# sindresorhus/ky：simplify-code 分析报告

- **仓库**：https://github.com/sindresorhus/ky
- **Commit**：`3541888878c4d303a8bc2ad61815f4e048f55a5d`
- **范围**：`source/utils/merge.ts`（501 行）、`source/utils/normalize.ts`（100 行）
- **项目约定**：`xo`（含 `max-depth: 4`、`complexity: 20`）、严格 `tsconfig`、`engines.node >= 22`
- **技能参考**：`rules.md` §2/§3/§9/§13/§19、`languages-tier1.md#TypeScript`、`behavior-traps.md`

## Summary
代码注释密集但几乎都是"为什么"（应保留）。主要问题是 `deepMergeInternal` 已是长函数（xo 报告圈复杂度 61，阈值 20），但拆分涉及合并顺序语义，只能**提议**。小处应用了 1 处安全简化；另一处"去嵌套三元"在本项目 lint 规则下反而引入新告警，已回退——这是本次很有代表性的"项目约定优先"案例。

## 代码在做什么
`deepMerge` 实现 `ky.extend()` 的选项合并：hooks 追加、headers 合并/删除标记、searchParams 累积、`context` 浅合并、`retry` 数字/对象互转、`replaceOption()` 包装器的识别与剥离；`normalizeRetryOptions` 校验 retry 配置并给出特定错误文本（公共错误信息）。

## 发现（按优先级）

### P1-1（仅提议）`deepMergeInternal` 长函数 / 按 key 分派 — `merge.ts:340-498`
- **规则**：§2 长函数、§9 "重复的类型/键 switch → 查找表"、§19 Long function → Extract Function
- **证据**：xo `complexity` 告警：`Arrow function has a complexity of 61. Maximum allowed is 20`；同一 `isRoot && key === 'retry'` 判断出现 3 次（420、431、439）。
- **建议**：把每个根级特殊键抽成具名处理函数，例如
  ```ts
  const rootKeyHandlers = {
    signal: mergeSignal, context: mergeContext, searchParams: mergeSearchParams, retry: mergeRetry,
  } satisfies Partial<Record<keyof Options, RootKeyHandler>>;
  // 循环体：if (isRoot && key in rootKeyHandlers) { state = rootKeyHandlers[key](state, value, isReplace); continue; }
  ```
  `retry` 的三段逻辑合并到一个 `mergeRetry` 中。
- **为何不直接改**：各分支之间有顺序依赖（如 `retry` 的校验 → 数字展开 → 递归合并，`headers === null` 检查必须在通用合并前），且 `signal` 分支在"清空"后还可能继续走 `continue`。没有针对每个键组合的特征测试前，属于高风险结构重构（golden rule 1、rules §14 先补特征测试）。
- **风险**：中；**需审批**（无公共 API 变化，但属于大重构）。

### P3-1 `mergeHooks` 三元选择首参 — `merge.ts:454-456`
- **规则**：§20.5 合并重复调用；与 465 行 `mergeHeaderContainers(isReplace ? {} : headers, …)` 保持一致
- **Before**：`hooks = isReplace ? mergeHooks({}, hookValue) : mergeHooks(hooks, hookValue);`
- **After**：`hooks = mergeHooks(isReplace ? {} : hooks, hookValue);`
- **风险**：极低；**行为保持**：是（已应用 + 测试）。

### P3-2（尝试后回退）嵌套三元 — `merge.ts:388-392`
- **规则**：SKILL golden rule 4 "不要嵌套三元"
- 原代码：`context: value === undefined ? {} : (isReplace ? {...value} : {...returnValue.context, ...value})`
- 尝试改成 `let context = {}; if (value !== undefined) { context = isReplace ? … : …; }` 后，xo 新增一条 `max-depth`（嵌套 5 层 > 4）告警。项目 lint 是法（golden rule 3、rules §13），因此**回退**。正确方向是先做 P1-1 把 `context` 处理抽到独立函数，届时嵌套三元可以自然展开。

### P3-3 `normalizeRetryOptions` 校验表里的标签推导 — `normalize.ts:63-71`
- `key === 'methods' ? 'strings' : 'numbers'` 把"期望类型"知识放在消息拼接处；可以把标签放进元组：`['methods', isString, 'strings']`。收益小，未应用。错误文本是公共契约，改写时必须逐字保持。

## 刻意保留
- 大量长注释：均解释"为什么"（跨副本 `Symbol.for`、`null` vs `undefined` 语义），属于 §7 应保留类。
- `cloneShallow`、`getReplaceState`：深模块，接口小。
- `retryMethods`/`retryStatusCodes` 等常量已命名，无魔法数问题（`0.3 * 2 ** n * 1000` 的退避公式是公开文档行为）。

## 验证
- 本机 Node 20，项目要求 Node ≥ 22：选定 6 个相关测试文件（merge/context/hooks/headers/normalize/retry-options）共 428 个用例，**修改前后均 418 pass / 10 fail**，失败集合逐条比对一致（均为 hooks 中依赖 Node 22 abort 语义的用例）。
- `tsc --project tsconfig.dist.json --noEmit`：通过。
- `xo source/utils/merge.ts`：告警数与基线相同（6 条，均为既有）。
- diff：`applied.diff`（+1/−3）。

## 置信度
中高。P1-1 的复杂度判断有 lint 数据支撑；未在 Node 22 上跑全量。
