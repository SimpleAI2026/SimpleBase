---
name: review-tests
description: Use when 审查测试切片质量、测缺口、金字塔失衡、结构重构测试、合并前测评审，或红绿循环后需要独立测向 code review；when reviewing test design rather than only running the suite.
---

# 测试评审（review-tests）

**原则：** 早评测、常评测。结构重构与缺口分析在此闸，不在红绿微循环里硬做。

父入口：[../../SKILL.md](../../SKILL.md)  
写测原则：[../../references/good-test-writing.md](../../references/good-test-writing.md)  
假测词典：[../../references/fake-test-patterns.md](../../references/fake-test-patterns.md)  
打假测：[../test-audit/SKILL.md](../test-audit/SKILL.md)  
完成验证：[../verification-before-claim/SKILL.md](../verification-before-claim/SKILL.md)

## 何时用本闸

- 任务/功能完成后的**测切片**评审（对标 requesting-CR 的测向）  
- 怀疑测结构差、horizontal 遗留、金字塔失衡  
- 需要**结构重构**测试或调整 seam  
- 合并前：行为测是否足以保护 diff  
- Agent 生成大量测试后的设计层审查（junk 细节可先交 test-audit）

## 与红绿的边界（本包定论）

| tdd-red-green | review-tests |
|---------------|--------------|
| RED→GREEN + 最小清理 | 改 seam、拆并测文件、换测策略、补缺口计划 |
| 单刀 vertical slice | 跨多文件的测架构决策 |
| 保持当前绿 | 重构前后单独 verification |

## 评审清单

### A. 证明力

- [ ] 每条关键需求/行为有独立证明（非 duplicate-owner 空转）  
- [ ] 曾见证 RED（或 audit 时 mutation 敏感）  
- [ ] 无 tautological / assertion-free / mock-as-SUT  
- [ ] 期望值可追溯到规格或算例  

### B. Seam 与耦合

- [ ] 测落在约定公共 seam  
- [ ] 无 implementation-coupled / change detector  
- [ ] 无 test-only seam；mock 仅在系统边界  
- [ ] 内部 rename 不应无故炸测  

### C. 形状与金字塔

- [ ] 多数快测在逻辑/单元层；边界测有明确理由  
- [ ] 端到端仅覆盖关键路径（本包不引入 Playwright）  
- [ ] DAMP：单测可读；名字=能力规格  
- [ ] 无 horizontal slicing 残骸（一大坨未驱动实现的想像测）  

### D. 缺口

- [ ] 错误路径 / 权限 / 空值 / 幂等 / 并发（若相关）  
- [ ] 回归：每个已修 bug 有 Prove-It 锁  
- [ ] 配置/迁移/安全契约是否有 owner（见 retention bar）  

## 评审输出格式

```markdown
## 测试评审

### 优点
- ...

### 问题
- Critical: ...（假绿、无 RED、缺主契约）
- Important: ...（耦合、缺口、金字塔）
- Minor: ...

### 假测标签
- <词汇> @ <文件:行> → 建议

### 结构重构建议（若需要）
- 目标 seam:
- 步骤与验证命令:

### 裁决
- [ ] 可继续 / 可合并
- [ ] 阻塞：须先修复 Critical/Important
```

## 行动规则

- Critical：立即修，再继续功能  
- Important：前进前修完  
- Minor：可记债，不假装不存在  
- 评审错了：用技术证据回推（附测输出），不靠资历  

## 衔接

1. Junk/删留细节 → 加载 test-audit  
2. 修复行为 → tdd-red-green（Prove-It）  
3. 声称修好/可合并 → verification-before-claim  
4. 通用代码 CR → 项目的 requesting-code-review；本闸专注**测**切片  

## 红旗

- 「测都绿了所以设计没问题」  
- 跳过评审因为「测很简单」  
- 在红绿循环里做大重构却不经本闸  
- 忽略 Critical 假绿继续加功能  
