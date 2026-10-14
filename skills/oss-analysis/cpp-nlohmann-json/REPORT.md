# nlohmann/json：simplify-code 分析报告

- **仓库**：https://github.com/nlohmann/json
- **Commit**：`7461a199b5ca4d7e43d463c7acee9256f481b04d`
- **范围**：`include/nlohmann/detail/string_escape.hpp`（108 行）、`value_t.hpp`（183 行）、`string_utils.hpp`（313 行）
- **约束**：C++11 起支持；模板参数 `StringType` 允许**用户自定义字符串类型**（有 `tests/src/unit-alt-string.cpp` 专门覆盖）
- **技能参考**：`languages-tier1.md#C++`、`rules.md` §6/§20、`behavior-traps.md`

## Summary
这三份文件注释详尽、实现精炼。**最值得记录的结论是一个"看似安全、实际会破坏 API"的简化**：把两次 `find_first_of(char)` 合并为一次 `find_first_of("~/")`。它在 `std::string` 下测试全过，但在项目的自定义字符串测试中**编译失败**，已回退。最终 0 处应用、2 条 P3 建议。

## 代码在做什么
`escape/unescape`：RFC 6901 JSON Pointer 的 `~0`/`~1` 转义，单遍重建。`value_t`：类型排序表与 `<=>`/`<` 运算符、整数与浮点无损比较。`string_utils`：UTF-8 编码、Hoehrmann DFA 解码、UTF-8 校验与清洗。

## 发现（按优先级）

### 已否决（验证失败）合并两次查找 — `string_escape.hpp:35-40`
```cpp
// 原代码
auto next_special = [&s](std::size_t from) {
    const auto tilde = s.find_first_of('~', from);
    const auto slash = s.find_first_of('/', from);
    return tilde < slash ? tilde : slash; // npos is the largest value
};
// 候选（已回退）
auto pos = s.find_first_of("~/", 0);
```
- 对 `std::string` 语义等价且只扫一遍；`unit-json_pointer.cpp`（200345 个断言）全过。
- 但 `unit-alt-string.cpp` 中的 `alt_string` 只提供 `find_first_of(char, size_t)`，候选写法报错 `invalid conversion from 'const char*' to 'char'`。`StringType` 的**最小接口**是库的隐式公共契约 → golden rule 2，否决。见 `rejected.diff`。
- 这个 lambda 看似"多余的封装"，其实是在最小接口约束下的最优写法。

### P3-1 `sanitize_utf8` 中"截断 + 追加 U+FFFD"出现两次 — `string_utils.hpp:276-282`、`301-306`
- **规则**：§6；但只有两份，且一份在循环内（还要更新 `result_len_after_last_accept`）、一份在循环后。
- 可抽成局部 lambda `drop_ill_formed()`，但需要捕获 3 个可变状态，可读性收益有限。**仅建议**。

### P3-2 `encode_utf8` 四个分支 — `string_utils.hpp:73-104`
- 可以用循环按 continuation 字节数生成，但现有展开写法与 UTF-8 规范表格一一对应，更易审计。**不建议改**，列出仅为说明已考虑。

## 刻意保留
- `int_to_string` + `to_string` 两层（string_utils.hpp:25-39）：`using std::to_string;` 是 ADL 定制点，供自定义 `StringType` 重载，不是浅包装。
- `operator<=>` / `operator<` 的 `#if` 双实现与 GCC bug 绕行（value_t.hpp:83-118）：编译器兼容需要。
- `compare_integer_with_float` 末尾三个 `if`（171-179）：合并成嵌套三元会违反 golden rule 4。
- `decode` 的 400 项表：算法本体。

## 验证
- 候选修改：`g++ -std=c++17 unit.cpp unit-json_pointer.cpp` → 3 test cases / 200345 assertions 全过；`unit.cpp unit-alt-string.cpp` → **编译失败**。
- 回退后：`unit-alt-string.cpp` → 69 assertions 全过。
- box 无 CMake，未跑全量测试（按需手工编译单个测试单元）。

## 置信度
高。
