# JamesNK/Newtonsoft.Json（`Utilities/StringUtils.cs`）：simplify-code 分析报告

- **仓库**：https://github.com/JamesNK/Newtonsoft.Json
- **Commit**：`52fa3aef1f2cadcd3a3f874251eddc98d3efbbaa`
- **范围**：`Src/Newtonsoft.Json/Utilities/StringUtils.cs`（371 行，`internal static class`）
- **约束**：多目标框架（net20 … net6.0+），大量 `#if HAVE_*` 条件编译；类为 `internal`，但通过 `InternalsVisibleTo` 暴露给测试程序集
- **技能参考**：`languages-tier1.md#C#`、`rules.md` §6/§8/§20、`behavior-traps.md`（LINQ 延迟执行）

## Summary
找到 1 个真正的重复知识（P2）和 4 个小 P3。box 无 .NET SDK，**未运行测试**，全部结论基于阅读。

## 代码在做什么
序列化器使用的字符串工具：格式化重载、驼峰/蛇形/烤串命名转换（`CamelCasePropertyNamesContractResolver`、`SnakeCaseNamingStrategy` 的核心）、代理对判断、`\uXXXX` 转义、按大小写宽容查找成员。

## 发现（按优先级）

### P2-1 `ToSeparatedCase` 重复了 `ToLower` 的条件编译 — `StringUtils.cs:258-264` vs `196-204`
- **规则**：§6 DRY（"如何做不变区域小写"是一条知识，已有私有助手 `ToLower`）
- **Before**
  ```csharp
  char c;
  #if HAVE_CHAR_TO_LOWER_WITH_CULTURE
                      c = char.ToLower(s[i], CultureInfo.InvariantCulture);
  #else
                      c = char.ToLowerInvariant(s[i]);
  #endif
                      sb.Append(c);
  ```
- **After**：`sb.Append(ToLower(s[i]));`
- **风险**：低（私有、同一实现）；**行为保持**：是（推理）。

### P3-1 `ToLower` 先赋值再返回 — `StringUtils.cs:196-204`
```csharp
private static char ToLower(char c)
{
#if HAVE_CHAR_TO_LOWER_WITH_CULTURE
    return char.ToLower(c, CultureInfo.InvariantCulture);
#else
    return char.ToLowerInvariant(c);
#endif
}
```

### P3-2 `CreateStringWriter` 两个单次临时变量 — `StringUtils.cs:113-119`
`return new StringWriter(new StringBuilder(capacity), CultureInfo.InvariantCulture);`

### P3-3 `ForgivingCaseSensitiveFind` 的 `else` 与延迟查询 — `StringUtils.cs:142-152`
- 可去掉 `return` 后的 `else`（§5）。
- **不要**为"避免重复枚举"而 `.ToList()`：`caseInsensitiveResults` 被 `Count()` 和 `SingleOrDefault()` 各枚举一次，`valueSelector` 的调用次数是可观察的（C# ⚠ LINQ 延迟执行）。物化会改变调用次数——若想优化应单独提议。

### P3-4 代理对范围魔法数 — `StringUtils.cs:293`、`302`
`55296..56319` / `56320..57343` 可写成 `'\uD800'..'\uDBFF'` 等字符字面量，自解释。仅在老框架的 `#else` 分支编译。

## 刻意保留 / 需审批
- `FormatWith` 四个重载 + 私有 `params` 版本（54-81）：注释说明是为了避免数组分配的有意设计。
- `IsHighSurrogate`/`IndexOf`/`Replace` 的 `#if` 包装：多目标框架兼容层，不是浅包装。
- `Trim` 中 `throw new ArgumentNullException()` 缺少参数名（340）：与同文件其它抛出不一致，但异常的 `ParamName`/消息是可观察行为 → 需审批。
- 扩展方法 `StartsWith(this string, char)`（324）：在新框架上与 BCL 同名重载重叠；改名属于 API 变化（虽是 internal，但测试程序集可见）。

## 验证
- 仅阅读与推理；box 无 `dotnet`，未编译、未运行测试。

## 置信度
中高（P2-1 的等价性是逐字符号对比得出）；未经编译器验证。
