---
name: tester-skill
description: Use when 审查/编写/验收测试、怀疑假测/假绿、TDD、完成前验证、测试缺口、mutation、Agent 生成测试不全、coverage 虚高、或需要决定删留测试；also when reviewing or accepting Agent-generated tests, hunting tautological/fake-green suites, or verifying before claiming done.
---

# Tester Skill（测试编排入口）

**核心原则：** 测试是证据（proof），不是装饰。假绿 = 未测。违反条文即违反精神。

对标综合：obra（TDD + verification + requesting-CR 测切片）、mattpocock（tdd 反模式）、openclaw（test-audit junk/retention）、addyosmani（TDD 写测原则 + Prove-It + 金字塔；本包**不含** Playwright/DevTools/browser）。

## 何时加载本 skill

- 写新功能 / 修 bug / 改行为之前或之中
- 验收、审查、合并前要判断测试是否真保护行为
- 怀疑 tautological、假绿、implementation-coupled、coverage 虚高
- Agent 刚批量生成测试，需打假测 / 找缺口
- 声称「完成 / 全绿 / 已修好」之前需要 fresh evidence
- 做 mutation 或故意破坏以验证测试敏感度

**不要用本包做：** 纯文档/静态资源改动、无行为影响的配置；浏览器 E2E 工具链（本包刻意不含 Playwright）。

## 四道闸（概览）

按场景选闸；可串联，不可用口号跳过。

| 闸 | 子 skill | 一句话 |
|----|----------|--------|
| 1. 红绿强制 | [subskills/tdd-red-green/SKILL.md](subskills/tdd-red-green/SKILL.md) | 先 RED 再 GREEN；最小实现；允许最小清理 |
| 2. 完成前验证 | [subskills/verification-before-claim/SKILL.md](subskills/verification-before-claim/SKILL.md) | 无本轮新鲜命令输出，不得声称通过 |
| 3. 打假测 / 留删 | [subskills/test-audit/SKILL.md](subskills/test-audit/SKILL.md) | 拒 junk；按 retention bar 留删；不追删除数 |
| 4. 测切片评审 | [subskills/review-tests/SKILL.md](subskills/review-tests/SKILL.md) | 结构重构、缺口、金字塔、与 CR 衔接 |

**Refactor 对齐（本包定论）：**

- 红绿强制；循环内只允许**最小清理**（命名、去重、提取小 helper），且必须保持绿。
- **结构重构**（改 seam、拆模块、重测布局）归 [review-tests](subskills/review-tests/SKILL.md)，不在红绿循环里硬塞大重构。

## 假测统一词汇

全文与报告必须用这些词（详见 [references/fake-test-patterns.md](references/fake-test-patterns.md)）：

`tautological` · `mirror assertion` · `implementation-coupled` · `change detector` · `mock-as-SUT` · `test-only seam` · `horizontal slicing` · `assertion-free` · `source-grep` · `duplicate-owner`

## 铁律与写测原则

- 铁律全文：[references/iron-laws.md](references/iron-laws.md)
- 好测写法（pyramid / DAMP / Prove-It / seam）：[references/good-test-writing.md](references/good-test-writing.md)

## 编排决策（简表）

| 你面对的情况 | 先加载 |
|--------------|--------|
| 要实现行为 / 修 bug | tdd-red-green（bug 用 Prove-It） |
| 想说「好了 / 全绿」 | verification-before-claim |
| 新测可疑 / 扫假测 / 决定删留 | test-audit |
| 测结构差、缺口、大重构测 | review-tests |
| 不确定是否假绿 | fake-test-patterns → 再 test-audit 或 mutation |

## 交付报告格式

完成一轮测试相关工作后，用此结构汇报（字段可空但勿省略标题）：

```markdown
## Tester 报告

### 范围
- 变更 / 子系统 / 测试文件：
- 约定 seams：

### 闸门结果
- [ ] RED→GREEN 证据（命令 + 失败原因 + 通过输出摘要）
- [ ] 本轮 verification 命令与 exit code
- [ ] 假测扫描：发现的模式（用统一词汇）与处置
- [ ] 留删决定（keeper 名 / 删除理由）

### 缺口与风险
- 未覆盖行为 / 建议 mutation：
- 结构重构是否移交 review-tests：

### 证据摘录
- 关键命令与关键输出（失败/通过各至少一行实质信息）
```

## 与其它 skill 的关系

| 关系 | 说明 |
|------|------|
| `test-driven-development` / obra TDD | 细节下沉到 `tdd-red-green`；本入口负责编排 |
| `verification-before-completion` | 对应 `verification-before-claim` |
| `requesting-code-review` | 测切片与结构问题走 `review-tests`，再衔接到 CR |
| 系统性调试 | bug 先 Prove-It 再生证据；不替代根因分析 |
| 本包边界 | **无** Playwright / DevTools / browser 段；E2E 工具另开 skill |

## 红旗 — 立刻停

- 实现先于失败测试；测试「一跑就绿」却说已验证
- 用「应该过了」「看起来没问题」代替命令输出
- 批量生成断言却不回答 authoring gate
- 为冲 coverage 加 assertion-free / mirror assertion
- 在红绿循环里做大型结构重构

**下一步：** 按上表加载对应 `subskills/*/SKILL.md`，细节以 references 为准。
