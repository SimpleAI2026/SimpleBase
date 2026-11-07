---
name: verification-before-claim
description: Use when 即将声称完成/通过/修好/全绿、表达满意、准备 commit/PR/交任务，或只有「应该过了」而没有本轮命令输出；before any completion claim.
---

# 完成前验证（verification-before-claim）

**原则：** 证据先于声称。无本轮新鲜验证，声称完成 = 不诚实。

父入口：[../../SKILL.md](../../SKILL.md)  
铁律：[../../references/iron-laws.md](../../references/iron-laws.md)

## 铁律

```
NO COMPLETION CLAIMS WITHOUT FRESH VERIFICATION EVIDENCE
```

本条消息里没跑验证命令并读完输出 → 不能说通过。

## Gate 函数

声称任何状态或满意之前：

1. **IDENTIFY** — 哪条命令能证明该声称？  
2. **RUN** — 完整、新鲜执行（不要用「上次」）  
3. **READ** — 读满输出；看 exit code；数失败  
4. **VERIFY** — 输出是否支撑声称？否 → 报告实际状态  
5. **ONLY THEN** — 带证据声称  

跳过任一步 = 未验证。

## 声称 → 证据

| 声称 | 需要 | 不够 |
|------|------|------|
| 测试通过 | 本轮测命令输出、0 failures | 上次运行、「应该过」 |
| 构建成功 | 构建命令 exit 0 | 仅 lint |
| bug 已修 | 复现测曾 RED 现 GREEN + 相关 suite | 「改了代码」 |
| 回归测有效 | 证实过 RED→GREEN（必要时 revert 探针） | 只看见一次绿 |
| 需求满足 | 对照清单逐项 | 「测都绿了」 |
| Agent 做完了 | 看 diff + 自跑验证 | 只信 Agent 口述 success |

## 回归测（与 TDD 衔接）

Prove-It / 新回归：

```
写测 → 跑（应 RED）→ 修复 → 跑（应 GREEN）
可选：暂时撤修复 → 必须再 RED → 恢复 → GREEN
```

未做 RED 见证就说「有回归保护」→ 无效。

## 红旗 — 停

- `应该` / `大概` / `看起来` / `Great!` / `Done!` 出现在验证之前  
- 准备 commit/PR 却未跑门禁命令  
- 信任子 Agent「success」而不看输出/diff  
- 「just this once」「我太累了」  
- 同一命令在**代码未变**时连跑两次当安慰（无增量信息）

## 借口 → 现实

| 借口 | 现实 |
|------|------|
| 我很有信心 | 信心 ≠ 证据 |
| lint 过了 | lint ≠ 编译/测 |
| 上次绿过 | 要本轮新鲜 |
| 部分检查够了 | 部分证明不了全部声称 |

## 输出模板（声称时附带）

```text
验证命令: <exact command>
exit code: 0
摘要: <n> passed / 0 failed（或失败列表）
结论: <可声称的范围>
```

若假绿可疑，先 [../test-audit/SKILL.md](../test-audit/SKILL.md) 或 mutation，再声称。
