# spf13/cobra：simplify-code 分析报告

- **仓库**：https://github.com/spf13/cobra
- **Commit**：`adbc8813901bba65827259daa8e22ff94ec1f30e`
- **范围**：`args.go`（全文）、`cobra.go`（全文）、`command.go` 的 `defaultUsageFunc`（1973-2040）
- **语言约束**：`go.mod` 声明 **`go 1.15`** —— 这直接否决了 `slices.Contains`、内建 `min/max` 等现代写法（languages-tier1 Go ⚠ "当模块 Go 版本允许时"）
- **技能参考**：`rules.md` §5/§6/§7/§20、`languages-tier1.md#Go`、`behavior-traps.md`、examples A7/A8/C4/C9

## Summary
cobra 的这部分代码整体已接近最小。应用了 2 处安全简化（嵌套 → 卫语句；三段重复的子命令打印循环合并），全部测试通过。另发现 1 处**疑似缺陷**（不是简化，单独列出需审批），以及若干"看起来能删、其实不能删"的点。

## 代码在做什么
`args.go` 提供公开的位置参数校验器（`NoArgs`、`ExactArgs`、`OnlyValidArgs` 等，错误文本是用户可见输出）；`cobra.go` 是全局开关、模板函数注册与若干小工具；`defaultUsageFunc` 是默认 usage 模板的手写等价实现（注释要求与模板同步修改）。

## 发现（按优先级）

### P2-1 `OnlyValidArgs` 主体包在 `if` 中 — `args.go:51-66`
- **规则**：§5 卫语句；Go "左对齐快乐路径"（examples A7）
- **Before**
  ```go
  if len(cmd.ValidArgs) > 0 {
      validArgs := ...
      for ... { if !stringInSlice(...) { return fmt.Errorf(...) } }
  }
  return nil
  ```
- **After**
  ```go
  if len(cmd.ValidArgs) == 0 {
      return nil
  }
  validArgs := ...
  for ... { if !stringInSlice(...) { return fmt.Errorf(...) } }
  return nil
  ```
- **风险**：低；**行为保持**：是（已应用 + 测试）。

### P3-1 `defaultUsageFunc` 三段重复的子命令打印循环 — `command.go:1991-2017`
- **规则**：§6 DRY（同一"可见性规则 `IsAvailableCommand() || Name()==help`"与同一行格式重复 3 次）
- **After**（谓词闭包，避免 rules B1 所说的布尔 flag 参数）
  ```go
  cmds := c.Commands()
  printSubcommands := func(inSection func(*Command) bool) {
      for _, subcmd := range cmds {
          if inSection(subcmd) && (subcmd.IsAvailableCommand() || subcmd.Name() == helpCommandName) {
              fmt.Fprintf(w, "\n  %s %s", rpad(subcmd.Name(), subcmd.NamePadding()), subcmd.Short)
          }
      }
  }
  // 调用：func(*Command) bool { return true } / sub.GroupID == group.ID / sub.GroupID == ""
  ```
- **注意**：闭包捕获循环变量 `group`，但在同一迭代内同步调用，Go < 1.22 的捕获陷阱不适用；`cmds` 仍只取一次，`Commands()` 的排序副作用次数不变。
- **取舍**：净行数 −1，但去掉了"可见性规则"的三份拷贝；由于函数注释要求与模板同步，维护者也可能更偏好与模板逐段对应的原写法 → 定级 P3（可选）。
- **行为保持**：是（已应用 + 测试）。

### 需审批（缺陷，非简化）`OnlyValidArgs` 建议词基于 `args[0]` — `args.go:61`
```go
return fmt.Errorf("invalid argument %q for %q%s", v, cmd.CommandPath(), cmd.findSuggestions(args[0]))
```
报错的是 `v`，"Did you mean…" 建议却基于第一个参数。输入 `app valid typo` 时，提示会针对 `valid` 而非 `typo`。改为 `findSuggestions(v)` 会改变用户可见输出 → 按 golden rule 1 单独提议，**未修改**。

### P3-2 `rpad` 两次 Sprintf — `cobra.go:174-177`
`fmt.Sprintf(fmt.Sprintf("%%-%ds", padding), s)` 可写作 `fmt.Sprintf("%-*s", padding, s)`。负数 padding 时两者行为需逐一核对（`%--3s` vs `*` 取负值），收益很小，**未应用**。

## 刻意保留
- `Gt`、`Eq`（cobra.go:114-157）：带 FIXME "v2 删除"，但是导出函数 → 公共 API，不能删。
- `appendIfNotPresent`（166-171）：未导出、包内无直接调用，看起来是死代码；**但它在 `templateFuncs` 中注册（cobra.go:36）供用户模板调用**——正是 rules §7 所说"注意动态注册"的例子，删除会破坏用户模板。
- `stringInSlice`（225-232）、`ld` 中手写 min（209-217）：`go 1.15` 下不能用 `slices.Contains` / 内建 `min`。
- `defaultUsageFunc` 与 `defaultUsageTemplate` 的"重复"：是有意的性能实现，注释要求同步，属于"不同原因变化"之外的同一知识的两种表示，保留。
- `legacyArgs`、`MatchAll` 等：已最小（examples C9 类型）。

## 验证
- `gofmt -l .`：无输出；`go vet .`：通过。
- `go test ./...`：修改前后均 `ok github.com/spf13/cobra`、`ok .../doc`。
- diff：`applied.diff`（+26/−25）。

## 置信度
高。
