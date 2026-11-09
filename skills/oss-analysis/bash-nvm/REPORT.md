# nvm-sh/nvm（`nvm.sh` 子集）：simplify-code 分析报告

- **仓库**：https://github.com/nvm-sh/nvm
- **Commit**：`b1b3a0038c0666c7436cb5d383608b4b72c8300b`
- **范围**：`nvm.sh` 第 1-140 行（输出/探测工具函数：`nvm_echo`、`nvm_cd`、`nvm_err`、`nvm_has*`、`nvm_command_info`、`nvm_has_colors`）；全文 5642 行未通读
- **约束**：**POSIX sh**，需在 sh/dash/bash/ksh/zsh 下工作（文件头注释）；ShellCheck（带文件级 disable）；测试框架 urchin
- **技能参考**：`languages-tier1.md#Bash/Shell`、`rules.md` §6/§20、`behavior-traps.md#Shell`

## Summary
应用了 1 处简化（`nvm_command_info` 中 `type` 最多被执行 5 次 → 1 次），官方单元测试在 bash 与 dash 下通过；但对比测试发现**它改变了一个可观察行为**：命令不存在时 stderr 从 5 行变为 1 行。按技能规则如实标注为"需审批"。另有 2 个 P3。

## 代码在做什么
这些函数是 nvm 的跨 shell 基础设施：在 zsh 中走 `builtin`、其它 shell 走 `command` 以绕过用户别名/函数；`nvm_command_info` 用于 `nvm debug` 输出某命令的真实来源。

## 发现（按优先级）

### P2-1（需审批）`nvm_command_info` 重复执行 `type` — `nvm.sh:93-111`
- **规则**：§6 / §20.3（同一昂贵/有输出的表达式不应重复求值）
- **Before**：每个 `if/elif` 都重新跑一次 `type "${COMMAND}" | …`，最坏 5 次。
- **After**
  ```sh
  local TYPE_INFO
  TYPE_INFO="$(type "${COMMAND}")"
  if nvm_echo "${TYPE_INFO}" | nvm_grep -q hashed; then
    INFO="$(nvm_echo "${TYPE_INFO}" | command sed -E 's/\(|\)//g' | command awk '{print $4}')"
  elif …   # 其余分支同理
  else
    INFO="${TYPE_INFO}"
  fi
  ```
- **等价性**：`$(...)` 去掉末尾换行，与原来把 `type` 输出直接管道给 grep/awk 的逐行处理一致；`local` 与赋值分行，不吞退出码；`type` 本身在子 shell 中运行，原写法也是（管道左侧），无哈希副作用差异。
- **可观察差异**：对不存在的命令，原实现向 stderr 打印 5 次 `type: X: not found`，新实现 1 次（stdout 完全相同）。这通常是改进，但属于输出变化 → 需维护者认可。
- **风险**：低-中；**已应用**到本地分支以便验证。

### P3-1 `[ -n "${ZSH_VERSION-}" ]` 重复 4 次，未复用已有的 `nvm_is_zsh` — `nvm.sh:16-18` vs `26`、`35`、`45`
同一知识已有具名函数。但 `nvm_echo` 是全脚本最热的函数，多一层函数调用在慢 shell 上有成本，且 `nvm_is_zsh` 可能被测试 mock —— 可选，不强推。

### P3-2 `nvm_has_colors` 内联 `[ -t 1 ]` — `nvm.sh:118`
与 `nvm_stdout_is_terminal`（20-22）重复。同上，可选。

## 刻意保留
- `nvm_echo`/`nvm_echo_with_colors`/`nvm_cd` 三份 `builtin`/`command` 分派（24-51）：三者格式串（`%s` / `%b`）和重定向不同，抽成通用 `nvm_builtin` 包装器需要额外参数，并在全脚本最热的路径上多一层函数调用；每份只有 5 行，收益不抵成本。
- `nvm_err` 的 `( … ) || return 0` 子 shell（56-58）：注释解释了 stderr 被关闭时 zsh 会中止的问题，**不能**简化掉。
- 未引入 `set -euo pipefail`、`[[ ]]`：文件是 POSIX sh（behavior-traps Shell 节：严格模式是行为变化，`[[` 不是 POSIX）。

## 验证
- 官方用例 `test/fast/Unit tests/nvm_command_info`：修改前后在 **bash 与 dash 下均 PASS**（box 无 zsh/ksh，未覆盖）。
- 自写对比脚本（wget、ls、cd、函数、不存在的命令；含 `hash` 后场景）：bash 下 stdout 一致、仅不存在命令的 stderr 不同（5→1 行）；dash 下完全一致。
- box 无 ShellCheck，未跑 lint。diff：`applied.diff`（+11/−9）。

## 置信度
中高（zsh/ksh 未测）。
