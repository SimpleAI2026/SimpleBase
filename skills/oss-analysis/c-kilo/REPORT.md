# antirez/kilo：simplify-code 分析报告

- **仓库**：https://github.com/antirez/kilo
- **Commit**：`323d93b29bd89a2cb446de90c4ed4fea1764176e`
- **范围**：`kilo.c` 第 366-865 行（语法高亮、行操作、文件读写），其余部分浏览
- **约束**：`cc -Wall -W -pedantic -std=c99`（Makefile），无测试套件
- **技能参考**：`languages-tier1.md#C`、`rules.md` §5/§7/§20、`behavior-traps.md`、examples A2/A15

## Summary
kilo 是 ~1300 行的教学型编辑器，风格紧凑。应用了 4 处行为保持的简化（死变量、布尔返回、两处去嵌套、冗余的 `if` + 重复赋值），用**自写的特征测试 harness** 证明输出逐字节一致。另外发现 1 个**真实缺陷**（`editorDelRow` 的 `idx` 更新方向错误，可导致越界读），按技能规则单独列为"需审批"，未修改。

## 代码在做什么
全局状态 `E` 持有行数组；每行有原始 `chars`、渲染后 `render`、高亮 `hl`。插入/删除字符或行后调用 `editorUpdateRow` 重建渲染与高亮；多行注释状态通过 `row->idx` 向后传播。

## 发现（按优先级）

### 需审批（缺陷）`editorDelRow` 中 `idx++` 应为 `idx--` — `kilo.c:628`
```c
memmove(E.row+at,E.row+at+1,sizeof(E.row[0])*(E.numrows-at-1));
for (int j = at; j < E.numrows-1; j++) E.row[j].idx++;   // 应为 idx--
```
- 删除一行后，后续行下标左移，`idx` 应减 1。用 `bug.c` 复现：插入 4 行后删除第 1 行，得到 `idx = 0, 3, 4`（期望 `0, 1, 2`）。
- **后果**：`editorUpdateSyntax` 用 `E.row[row->idx-1]` / `E.row[row->idx+1]` 判断多行注释，`idx` 偏大时会读到错误行，甚至越过 `numrows`（越界读）。
- 这是修 bug，不是简化（SKILL 明确"不用于 bug 修复"），因此只提议。

### P2-1 死变量 `nonprint` 与重复的分配大小表达式 — `kilo.c:557-573`
- **规则**：§7 删除死代码；§6 同一知识（分配大小）写了两遍
- `nonprint` 初始化为 0 后从未修改，`nonprint*9` 恒为 0；`allocsize` 算好后，`malloc` 又重写了一遍同样的表达式。
- **After**
  ```c
  unsigned int tabs = 0;
  ...
  unsigned long long allocsize = (unsigned long long) row->size + tabs*8 + 1;
  if (allocsize > UINT32_MAX) { ... exit(1); }
  row->render = malloc(allocsize);
  ```
- **注意**：原 `malloc(row->size + tabs*8 + nonprint*9 + 1)` 以 `int/unsigned` 运算，`allocsize` 为 `unsigned long long`；由于上面已保证 `≤ UINT32_MAX`，传给 `malloc(size_t)` 的值在 64 位平台相同（C ⚠：整型提升需逐一核对）。
- **行为保持**：是（已应用 + harness 验证）。

### P3-1 `if (cond) return 1; return 0;` — `kilo.c:373-378`
- **规则**：§9 / examples A2。C 中 `&&`/`||` 结果本身就是 `int` 0/1，`return cond;` 完全等价。
- **行为保持**：是（已应用）。

### P3-2 两处可去掉的嵌套 — `kilo.c:420-432`、`443-462`
- `if (...) { ...; continue; } else { ...; continue; }` → 去掉 `else`；`} else { if (quote) {...} }` → `} else if (quote) {...}`。
- **行为保持**：是（已应用）。

### P3-3 `editorInsertChar` 冗余判断 — `kilo.c:706-714`
```c
// Before
erow *row = (filerow >= E.numrows) ? NULL : &E.row[filerow];
if (!row) { while (E.numrows <= filerow) editorInsertRow(E.numrows,"",0); }
row = &E.row[filerow];
// After
while (E.numrows <= filerow) editorInsertRow(E.numrows,"",0);
erow *row = &E.row[filerow];
```
`row != NULL` 时 `filerow < numrows`，`while` 条件本就为假，所以外层 `if` 和第一次赋值都是多余的。**行为保持**：是（已应用）。

### P3-4（仅建议）`editorRowDelChar` 的时间耦合 — `kilo.c:694-700`
先 `editorUpdateRow(row)` 再 `row->size--`，导致渲染包含被移动的 `'\0'`；调用方 `editorDelChar`（791 行）随后又 `editorUpdateRow` 一次把它修正。把 `size--` 提到更新之前即可去掉第二次重建，但这改变了调用协议（rules §16 Temporal coupling），需与 P3-3 一起整体评审，未应用。

### P3-5（仅建议）颜色魔法数 — `kilo.c:520-531`
`return 36; /* cyan */` 等可改成 `enum { ANSI_RED = 31, ... }`。注释已说明含义，收益小。

## 刻意保留
- `editorSave` 的 `goto writeerr`（830-852）：C 的惯用单一清理出口（languages-tier1 C 节明确"不要把它简化掉"）。
- `editorInsertNewline` 的 `goto fixcursor`：同上，局部、清晰。
- `editorUpdateSyntax` 的整体状态机：长但线性，拆分会引入大量共享指针参数（`p`、`i`、`prev_sep`）。

## 验证
- 无官方测试，按技能第 2 步补了**特征测试**：`harness.c` 以 `#include "kilo.c"` 方式链接，加载两份输入文件，执行一组插入/换行/删除（含越过 EOF 的插入），dump 每行 `render|hl|hl_oc|size`。
- 原代码与修改后代码在两份输入上的输出 **md5 完全相同**（`e08ec511…`、`eb52dedc…`）。
- `cc -Wall -W -pedantic -std=c99`：前后均 0 告警。
- diff：`applied.diff`（+17/−24）；`bug.c` 为缺陷复现（需与原始 `kilo.c` 副本 `input.c` 一同编译）。

## 置信度
高（字节级特征测试）；缺陷结论高（有复现）。
