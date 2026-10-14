# square/okio（`internal/Path.kt`）：simplify-code 分析报告

- **仓库**：https://github.com/square/okio
- **Commit**：`a6ae13ee0e7522674ae6382aaf8e6d6e9ef4bb9e`
- **范围**：`okio/src/commonMain/kotlin/okio/internal/Path.kt`（404 行，全文）
- **约束**：Kotlin Multiplatform（JVM/JS/Native/Wasm）；`inline` + `@Suppress("NOTHING_TO_INLINE")` 是跨平台 `expect/actual` 共享实现的约定；Spotless/ktlint
- **技能参考**：`languages-tier1.md#Kotlin`、`rules.md` §6/§9/§20

## Summary
路径解析逻辑密集但组织良好。找到 1 个 P2（同一"盘符字母"规则写了 3 遍）和 2 个 P3。多平台 Gradle 构建过重，**未运行测试**。

## 代码在做什么
`Path` 的公共实现：根长度识别（`/`、`\`、`\\server`、`C:\`）、分段、父路径、`resolve`、`relativeTo`，以及 `Buffer.toPath(normalize)` 的规范化（`.`/`..` 处理、UNC 主机名不可弹出）。

## 发现（按优先级）

### P2-1 "ASCII 字母盘符"判断重复 3 次 — `Path.kt:96-97`、`119-120`、`402-403`
- **规则**：§6 DRY（一条 Windows 规则，三处必须同改）
- **After**
  ```kotlin
  private fun Byte.isAsciiLetter(): Boolean {
    val c = toInt().toChar()
    return c in 'a'..'z' || c in 'A'..'Z'
  }
  // rootLength:      if (!bytes[0].isAsciiLetter()) return -1
  // commonVolumeLetter: if (!bytes[0].isAsciiLetter()) return null; return bytes[0].toInt().toChar()
  // startsWithVolumeLetterAndColon: return get(0).isAsciiLetter()
  ```
- **注意**：不要换成 `Char.isLetter()`——它接受非 ASCII 字母（behavior-traps：标准库函数语义不完全相同就不能替换）。
- **风险**：低；**行为保持**：是（推理）。

### P3-1 混合 `||`/`&&` 无括号 — `Path.kt:359`
`!normalize || !absolute && (canonicalParts.isEmpty() || canonicalParts.last() == DOT_DOT)`
`&&` 优先级更高，语义正确，但需要读者心算。加括号 `!normalize || (!absolute && (...))`，纯可读性、零行为变化。

### P3-2 `commonParent` 的 `when` 每支都 `return` — `Path.kt:152-174`
可改为 `return when { ... }` 并把内部 `if` 改成表达式，例如 `lastSlash == 2 && volumeLetter != null -> if (bytes.size == 3) null else Path(bytes.substring(endIndex = 3))`。收益中等，注释（"C:\ has no parent"）需保留在各分支；可选。

## 刻意保留
- 大量单行 `inline fun commonXxx()`：是 KMP 下 `Path` 各平台 `actual` 共享实现的入口，不是浅包装（rules §3 的例外："提供扩展/平台边界"）。
- `toPath` 中空分支 + 注释（357-358、361-362）：显式记录"静默吞掉 `..`"和"UNC 主机不可弹出"两条规则，比合并条件更清晰。
- `val part: ByteString; if … else …`（348-354）：可改成 `if` 表达式加 `.also { readByte() }`，但会把副作用藏进链式调用（golden rule 4），保留。
- `toSlash()` 两个重载（382-396）：输入类型不同（String vs Byte），合并需要转换，无收益。

## 验证
- 仅阅读与推理；box 有 JDK 21 但 okio 多平台构建需下载 Kotlin/Native 等工具链，判定为"过重"而跳过。

## 置信度
中高。
