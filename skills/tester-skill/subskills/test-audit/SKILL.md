---
name: test-audit
description: Use when 打假测、审计低价值测、决定测试删留、扫 tautological/mock-as-SUT/test-only seam/duplicate-owner、或 Agent 新写测需过 authoring gate；when sweeping junk tests or retention decisions.
---

# 测试审计（test-audit）

**原则：** 拒 junk；留独立契约证明；不追删除数量。对标 openclaw test-audit（junk + retention），词汇统一见 fake-test-patterns。

父入口：[../../SKILL.md](../../SKILL.md)  
假测词典：[../../references/fake-test-patterns.md](../../references/fake-test-patterns.md)  
铁律：[../../references/iron-laws.md](../../references/iron-laws.md)

## 两种模式

1. **Authoring gate** — 每条新/改测试上线前  
2. **Audit / sweep** — 成批审查既有测；按 owner 批次处理  

（整子系统 campaign 可另开清单；本文件给通用流程。）

## Authoring gate（四问）

写测前必须回答：

1. 保护的是哪条**可观察行为 / 不变量 / 独立契约**？  
2. 怎样的**可信回归**会让它失败？  
3. **为何现有证明不够？** 每条契约一个主 owner（最强边界）；另一层必须有不同传输/生命周期风险。优先扩展表驱动/共享 fixture，而非复制测。  
4. 是否新增**无生产调用方**的 export/flag/wrapper？（test-only seam）→ 证明移到真边界。

任问答不上 → 不写这条测。

## Junk（一律拒 / 审计猎杀）

用统一词汇标注（详见 fake-test-patterns）：

- tautological / mirror assertion / subject-generated expected values  
- implementation-coupled / call-shape / change detector  
- assertion-free；「不抛错」却无抛错路径  
- source-grep（锁标识符；行为变仍绿）  
- duplicate-owner；copied inventory  
- test-only seam  
- mock-as-SUT；fixture 伪造 owner 回执/顺序  
- negative control 失靶；name overclaim  

## Retention bar（保留）

独立强制以下契约时**保留**：公共 API / SDK / 协议 / 配置 / 迁移 / 存储 / 安全 / 平台默认 / 生成物跨语言 / 包与发布 / 架构约束 / **可观察**顺序。

另保留：

- 有可信失败模式的回归  
- 源码检查若是**最便宜的独立守卫**，且失败条件是用户可见键/字节/路径变化，并能活过 **identifier-only** 改名  
- 基线就失败的测 → 先当产品 bug 复现，勿直接当删除候选  

**静态或慢 ≠ 可丢。** 看起来像实现耦合的测，仍可能是唯一证明——删前必须指出更强 keeper。

## Audit 工作流

1. 读全测 + 生产 owner + 调用方/被调方 + 重叠测 + 简史  
2. 每条候选记录：  
   - 测名与位置；实际能抓什么失败  
   - 该 seam 的非测调用方  
   - **留下的更强证明（keeper 名）**，或证明无契约  
   - 删除解锁的生产/测试辅助代码  
   - 若有：覆盖率变化（注意：行覆盖存活 ≠ 断言仍在）  
3. 缺字段 → 候选未就绪；宁可少删高置信，不为凑数  
4. 删除 test-only 生产 seam 时与测一同处理  
5. 跑 owner/兄弟证明 + 变更相关检查；需要时 mutation  
6. 再过 verification-before-claim  

## 覆盖率陷阱

- 删「唯一断言」后行覆盖仍绿 → 别的测只执行行不检查  
- 总覆盖隐藏子系统失证 → 按被碰生产文件对比  
- 「零独有行」的测仍可能握有唯一断言 → 独有行只是候选信号，**命名 keeper** 才是删除依据  

## 报告字段

```markdown
### test-audit
- 模式: authoring | audit
- junk 发现: <词汇> @ <位置> → 处置
- 保留: <契约> ← <keeper 测名>
- 删除: <测名> ← keeper=<...>；理由=
- test-only seam: <符号> → 删/保留(维护者决策)
- 验证命令与结果:
```

结构问题、大面积重测 → [../review-tests/SKILL.md](../review-tests/SKILL.md)。
