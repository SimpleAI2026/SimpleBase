# 好测写法（综合原则）

对标 addyosmani 写测原则 + Prove-It + 金字塔概念，以及 mattpocock seam / vertical slice；**不含**浏览器/Playwright/DevTools。

## 测什么：行为与契约

好测读起来像规格：`"user can checkout with valid cart"`，不写 `"checkout calls paymentService.process"`。

- 测 **WHAT**（可观察结果），不测 **HOW**（内部步骤）
- 代码可整页重写；测应尽量仍绿
- 每个测一个行为概念；名字里有「and」就考虑拆分

## Seam（测试落点）

**seam** = 不伸进内部即可观察行为的公共边界。

写测前先列出 seams，并与用户/规格确认：

- 每个 seam 一行：捕获什么、漏什么
- 未确认的 seam 不写新测
- Mock：只在系统边界（外部 API、时钟、随机等），不 mock 自家模块当 SUT

## Vertical slice（反对 horizontal slicing）

```
一测（RED）→ 最小实现（GREEN）→ 最小清理 → 下一测
```

禁止：先堆全部测试再写实现。每一刀是 tracer bullet，根据上一循环学到的再开下一刀。

## Prove-It（修 bug）

```
bug 报告 → 先写复现测（必须 RED）→ 修 → 该测 GREEN → 全量相关 suite
```

禁止先改代码再补测。复现测是回归锁。

## 测试金字塔（概念）

投入大致倾向（按项目裁剪，不是KPI）：

```
        /  E2E / 端到端关键路径（少）  \
       /  Integration / 边界与协作（中） \
      /  Unit / 纯逻辑与快测（多）       \
```

- **多数**：快、小、确定性（纯逻辑、单进程）
- **部分**：跨真实边界（API、DB、FS）——仍优先真实或 in-memory fake
- **少数**：端到端关键路径（本包不规定浏览器工具）

**Beyoncé rule（概念）：** 你在乎的行为，就该有测；基础设施升级/他人重构没有义务替你抓 bug。

### 尺寸直觉

| 尺寸 | 约束 | 速度 |
|------|------|------|
| Small | 单进程、无外网/DB | ms |
| Medium | 可多进程、localhost | s |
| Large | 外部依赖允许 | min 级 |

## 断言与结构

### 状态优于交互

断言操作**结果**；少断言内部方法调用次序（易变 implementation-coupled）。

### DAMP over DRY（测里）

测试里允许重复，换取每条测自包含可读。过度共享 setup 会掩盖「本测到底证什么」。

### Arrange–Act–Assert

显式三段；Act 通常一行。一概念一断言组。

### 期望值独立源

字面量、手工算例、规格。禁止用被测代码重算期望（见 fake-test-patterns：tautological）。

## Test double 偏好序

1. 真实实现  
2. Fake（内存版依赖）  
3. Stub（固定返回）  
4. Mock（交互验证）— 最少用  

崩溃/回滚等：要打到真边界；「注入的假异常」≠ 进程终止；「mock 的 transaction」≠ DB rollback。

## 先发现仓库测栈

TDD 循环通用；命令不通用。写第一测前确认：

- 语言与构建清单（`go.mod` / `package.json` / …）
- 仓库包装脚本（`make test`、`./gradlew` 等）
- 单测 vs 全量命令
- 现有测的位置与命名惯例

循环中用**聚焦**命令；完成前用仓库的**全量/门禁**命令（见 verification-before-claim）。

## 最小清理 vs 结构重构

| 红绿内允许 | 交给 review-tests |
|------------|-------------------|
| 命名、去重、小提取 | 换 seam、拆测文件策略、测架构调整 |
| 保持绿 | 重构前后单独 verification |

## 写测前四问（authoring gate 摘要）

1. 保护的可观察行为 / 不变量 / 独立契约是什么？  
2. 什么样的真实回归会让它红？  
3. 为何现有证明不够？是否 duplicate-owner？  
4. 是否引入 test-only seam？若是 → 改到真边界。

完整闸门见 `subskills/test-audit/SKILL.md`。
