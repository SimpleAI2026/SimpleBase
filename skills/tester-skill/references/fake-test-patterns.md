# 假测模式词典（统一词汇）

审查、审计、报告时**只用**下列术语命名问题。对标 mattpocock + openclaw junk 列表，并与 obra/addy 反模式对齐。

## 速查表

| 术语 | 含义 | 典型征兆 | 处置 |
|------|------|----------|------|
| **tautological** | 期望值用与实现相同的算法/路径算出，测永远同意代码 | `expect(add(a,b)).toBe(a+b)`；`expected = fnUnderTest(x)` | 换成独立字面量/算例/规格 |
| **mirror assertion** | 断言只是把输入、常量或 fixture 原样镜面反射 | `expect(x).toBe(x)`；断言等于测试里刚赋的同一引用 | 删或改为独立期望 |
| **implementation-coupled** | 绑内部协作方、私有方法、调用形状或旁路查询 | mock 内部模块；`toHaveBeenCalledWith` 打内部；直查 DB 绕过接口 | 改到公共 seam；断言状态/结果 |
| **change detector** | 任何重构（含改名）就红，行为未变也红 | 脆快照、完整 call 序列、内部字符串 | 收窄到用户可见契约；能活过 identifier-only rename |
| **mock-as-SUT** | mock/fake 实现了被断言的行为，真 SUT 没被证明 | mock 返回「正确业务结果」再断言同一结果 | mock 只挡边界；行为由真代码或真 fake 产生 |
| **test-only seam** | 仅为测试存在的 export/flag/wrapper/注入点，无生产调用方 | 生产参数永远默认；唯一调用方在测试 | 证明移到真实边界；删除该 seam |
| **horizontal slicing** | 先写一大批测再写实现 | 批量想像行为；测结构先于理解 | 改为 vertical slice：一测→一最小实现→重复 |
| **assertion-free** | 无有效断言，或仅有「不抛错」而路径本不能抛 | 只 `render()`；空 `expect`；覆盖探针 | 补独立断言或删除 |
| **source-grep** | 用读源码/正则锁标识符，行为改了仍绿、改名却红 | `readFileSync` + `toContain('fooBar')` 锁符号名 | 仅当锁的是**用户可见**契约键/字节且能活过纯改名时保留 |
| **duplicate-owner** | 同一契约在多层各写一遍主回归 | 单元+集成+E2E 断言同一句话无额外风险 | 留最强边界一个 owner；其它层要有独特风险 |

## 相关 junk（归入上表或组合标注）

- **copied inventory**：测试里复制并断言与生产相同的导出清单/清单副本 → tautological + duplicate-owner
- **fixture-fabricated receipt/order**：本应由 owner 产生的回执/顺序由 fixture 伪造 → mock-as-SUT / tautological
- **negative control 失靶**：负面用例因别的守卫失败而「通过」→ 名称与断言不符，按 assertion 质量论处
- **name overclaim**：标题承诺多于断言实际检查 → 以断言为准改名或补断言

## 快速自检（写测 / 审测）

1. 若把生产实现改成恒错结果，本测会红吗？（否 → 假绿）
2. 期望值能否在不打开实现的情况下写下来？（否 → 疑 tautological）
3. 只 rename 内部符号，本测会红吗？（是且行为不变 → change detector / implementation-coupled）
4. 去掉这个 mock，断言还在测真代码吗？（否 → mock-as-SUT）
5. 仓库里是否已有更强边界的同类断言？（是 → duplicate-owner）

## mutation 探针（可选但有力）

对可疑测：故意破坏对应生产行为（返回错值、删分支），只跑该测：

- 仍绿 → 假测，按上表定性并重写/删除
- 按预期原因变红 → 保留，记入报告

## 报告时怎么写

```text
发现: tautological + mirror assertion @ foo.test.ts:42
处置: 期望值改为规格字面量 15；已 mutation 验证
发现: test-only seam `export __testOnly_reset`
处置: 删除 export；证明改到 public reset API
```
