# google/guava（`com.google.common.base` 子集）：simplify-code 分析报告

- **仓库**：https://github.com/google/guava
- **Commit**：`2b86a8216627a807ce8520abc6ae55437dbb85fb`（浅克隆 + sparse checkout）
- **范围**：`guava/src/com/google/common/base/Strings.java`（319 行）、`Joiner.java`（前 330 行：`Joiner` 主体与 `useForNull/skipNulls`）
- **约束**：`@GwtCompatible`，同时发布 JRE 与 Android 变体（Java 8 字节码/API 级别）；google-java-format；Error Prone
- **技能参考**：`languages-tier1.md#Java`、`rules.md` §3/§7/§20、examples C7/C9

## Summary
**未发现值得修改的问题（0 条 P1/P2，0 条 P3 需动手）。** 这两份文件是"已经最小"（技能的 *Already simple* 模式）的典型：几乎每个看似可以现代化/合并的地方都有兼容性或性能理由。按技能要求，此处如实报告"无改动"，不制造 churn。

## 代码在做什么
`Strings`：null/空串规范化、补齐、`repeat`、公共前后缀（不切断代理对）、`lenientFormat`（永不抛异常的格式化）。`Joiner`：不可变的拼接器，`skipNulls/useForNull` 通过匿名子类覆写实现。

## 发现
无需修改的发现。以下为审查过的候选及否决理由（供参考）：

| 候选 | 位置 | 为什么不改 |
|---|---|---|
| `padStart/padEnd` 循环 → `String.repeat` | `Strings.java:93-135` | Java 11 API；guava 需支持 Java 8 / Android / GWT。`repeat` 本身就标注 `@InlineMe(... "Java 11+ API only")` |
| `repeat` 手写倍增 arraycopy | `Strings.java:151-175` | 性能实现，注释要求同步更新 benchmark；溢出抛 `ArrayIndexOutOfBoundsException` 是可观察契约（behavior-traps：隐藏上限即行为） |
| `List<?> partsList = Arrays.asList(parts)` 单次使用临时变量 ×3 | `Joiner.java:125, 172, 260` | 临时变量是为了挂 `@SuppressWarnings("nullness")`（带 bug 号 TODO），内联会丢失抑制范围 |
| `skipNulls().appendTo` 两个 while 循环 | `Joiner.java:313-326` | "先找首个非 null，再带分隔符追加"是最直白的写法；改成布尔 `first` 标志并不更简单 |
| `join(Iterable)` 的 List 特殊路径 + 扩容 | `Joiner.java:207-232` | 长注释解释了 `size()` 可能过期/有副作用；属于有意的优化 |
| `commonPrefix/commonSuffix` 对称代码 | `Strings.java:184-220` | 两份、方向相反，抽象需要方向参数（rules §6 "错误抽象"） |
| `lenientToString` 捕获 `Exception` | `Strings.java:302-318` | 方法契约就是"永不抛出"，并且记录日志，不是 silent catch |

## 刻意保留
见上表。另外 `Strings.nullToEmpty` 等一行转发到 `Platform` 的方法：是 GWT/JRE 双实现的"超源"切换点，不是浅包装。

## 验证
- 仅静态阅读；box 无 Maven/Gradle 构建 guava 的条件（且没有改动需要验证）。

## 置信度
高（结论是"无改动"，主要风险是漏看；范围仅 2 个文件）。
