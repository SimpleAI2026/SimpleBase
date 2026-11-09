# 铁律（Iron Laws）

本文件被主入口与各子 skill 引用。违反条文 = 违反精神。

## 1. 无失败测试，无生产代码

```
NO PRODUCTION CODE WITHOUT A FAILING TEST FIRST
```

先写了实现？**删除**。重来。

禁止借口：

- 留作「参考」再「改编」
- 「太简单不用测」
- 「先探索一下」→ 探索代码可丢，正式实现必须 RED 先行
- 「测后补也一样」→ 测后回答「代码做了什么」；测先回答「应该做什么」

## 2. 无新鲜验证证据，无完成声称

```
NO COMPLETION CLAIMS WITHOUT FRESH VERIFICATION EVIDENCE
```

本轮消息里没有跑完整验证命令并读完输出，就不能说「通过 / 完成 / 修好了」。

禁止：`应该`、`大概`、`看起来`、引用上一次运行、只信 Agent「success」口头报告。

## 3. 期望值必须独立于被测实现

期望值来自：**字面量、手工算例、规格/契约**，不得来自：

- 用被测函数/同一算法再算一遍（tautological / mirror assertion）
- mock 里亲手实现的「正确答案」（mock-as-SUT）
- 测试自己定义又断言相等的清单副本

## 4. 只在约定 seam 上测

**seam** = 可观察行为的公共边界，不伸进内部。

未与用户/规格确认的 seam 上，不写新测。Mock 仅用于系统边界（外部 API、时间、随机、必要时 FS/DB），不 mock 自己的模块当 SUT。

## 5. 假绿不算绿

下列任一成立，则「全绿」无效：

- 从未见证过针对正确原因的 RED
- 断言不检验可观察行为（assertion-free）
- 改坏生产行为测试仍绿（对 mutation 不敏感）
- 仅靠 source-grep / change detector 维持通过

## 6. 一条契约一个主 owner

同一行为契约只在一个最强边界设主回归；其它层需有**不同**的传输/生命周期风险才可加测。禁止 duplicate-owner 层层复读。

## 7. 留删看 retention，不看删除 KPI

审计以「还剩什么独立证明」为准，不以「删了多少」为准。基线失败可能是产品 bug，先复现再决定删测。

## 与 Refactor 的边界

| 允许（红绿循环内） | 移交 review-tests |
|--------------------|-------------------|
| 改名、去显而易见重复、抽小 helper | 改模块边界 / seam、大挪测、换测策略 |
| 每步后保持全绿 | 结构重构前后需要独立 verification |

## 红旗速查

- 代码先于测试；测试立刻通过且说不清本该失败的原因
- 「spirit not ritual」「just this once」「already manually tested」
- coverage 上升但断言真空
- 为测而加的 production export / flag（test-only seam）
