---
name: tdd-red-green
description: Use when 实现新行为、修 bug、改既有逻辑，或用户提到 TDD / RED / GREEN / Prove-It / test-first；before writing production code for a behavior change.
---

# TDD 红绿循环（tdd-red-green）

**原则：** 没看见测因正确原因失败，就不知道它在测对的东西。

父入口：[../../SKILL.md](../../SKILL.md)  
铁律：[../../references/iron-laws.md](../../references/iron-laws.md)  
假测词典：[../../references/fake-test-patterns.md](../../references/fake-test-patterns.md)  
写测原则：[../../references/good-test-writing.md](../../references/good-test-writing.md)

## 铁律

```
NO PRODUCTION CODE WITHOUT A FAILING TEST FIRST
```

实现已先写？删除。不要「参考」「改编」。

## 循环

```
RED（写一条失败测）→ 确认因「缺行为」而失败
  → GREEN（最小代码使该测通过）
  → 最小清理（可选，保持绿）
  → 下一条 vertical slice
```

### RED

- 一条行为；名字像规格
- 在**已确认 seam** 上写（见 good-test-writing）
- 期望值来自独立源（字面量/算例/规格），禁止 tautological
- 跑聚焦测试命令，确认：**失败**（不是报错），且原因符合预期

立刻通过？你在测已有行为或假测 → 修正测试。  
报错（import/语法）？先修到「干净的失败」。

### GREEN

- 只写让**当前失败测**通过的最小代码
- 不预演下一测、不加投机特性（YAGNI）
- 再跑：本测绿，相关已有测仍绿，输出干净

### 最小清理（循环内允许）

允许：命名、去显而易见重复、抽小 helper。  
禁止：改模块边界、重铺测结构、大搬家 → 移交 [../review-tests/SKILL.md](../review-tests/SKILL.md)。

每步清理后必须仍绿。

## Prove-It（bug）

1. 写复现测 → 必须 RED（确认 bug 存在）  
2. 最小修复 → 该测 GREEN  
3. 跑相关/全量 suite（见 verification-before-claim）  
禁止：先修再补测。

## Vertical vs horizontal

一次一条 tracer bullet。禁止 horizontal slicing（先堆全部测再实现）。

## Mock

仅系统边界。禁止 mock-as-SUT、禁止拿内部协作方当断言中心（implementation-coupled）。

## 常见借口 → 现实

| 借口 | 现实 |
|------|------|
| 太简单 | 简单也会破；30 秒测 |
| 测后补一样 | 测后=做了什么；测先=该做什么 |
| 已手工测 | 无记录、不可回归 |
| 删掉可惜 | 沉没成本；无证明的代码是债 |
| 先留作参考 | 你会改编 = 测后；删除就是删除 |

## 红旗 — 停并重来

- 代码先于测；测立刻绿且说不清失败理由  
- 「just this once」「spirit not ritual」  
- 批量测再批量实现  
- 为测新增无生产调用方的 API（test-only seam）

## 完成前

加载 [../verification-before-claim/SKILL.md](../verification-before-claim/SKILL.md)。新测可疑时过 [../test-audit/SKILL.md](../test-audit/SKILL.md) 的 authoring gate。
