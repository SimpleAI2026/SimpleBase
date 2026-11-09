# BurntSushi/ripgrep（crate `grep-cli`）：simplify-code 分析报告

- **仓库**：https://github.com/BurntSushi/ripgrep
- **Commit**：`3fce3b5bb0236da2df6d99672afb8a719642eca7`
- **范围**：`crates/cli/src/` 下 `human.rs`、`pattern.rs`、`escape.rs`、`process.rs`（全文），以及 `decompress.rs`、`hostname.rs` 中的相关行
- **约束**：workspace `edition = "2024"`，`rust-version = "1.96"`；`rustfmt.toml`：`max_width = 79`
- **技能参考**：`languages-tier1.md#Rust`、`rules.md` §20、`behavior-traps.md`（错误文本/类型）、examples A9

## Summary
BurntSushi 的代码基本就是"最小可读等价物"。唯一系统性的冗余是 14 处 `io::Error::new(io::ErrorKind::Other, x)`，可用 `io::Error::other(x)`（1.74 稳定，远低于 MSRV）替换；另有 2 处 `match` 取 Option 可改 `let … else`。已应用并通过 crate 测试。

## 代码在做什么
`grep-cli` 为 ripgrep 提供 CLI 周边：人类可读大小解析、模式读取（UTF-8 校验 + 行号定位错误）、转义/反转义、子进程读取器（含异步读 stderr 防死锁）、解压命令选择、主机名。

## 发现（按优先级）

### P3-1 `io::Error::new(io::ErrorKind::Other, …)` ×14 → `io::Error::other(…)`
- **位置**：`pattern.rs:41, 86-89, 92-95, 108, 152`；`process.rs:76`；`human.rs:67`；`decompress.rs:60, 66, 464-467, 487`；`hostname.rs:28-31, 52, 70`
- **规则**：§20.4 用标准库现成操作（clippy 的 `io_other_error` lint 也指向此写法）
- **Before**：`io::Error::new(io::ErrorKind::Other, format!("{}: {}", path.display(), err))`
- **After**：`io::Error::other(format!("{}: {}", path.display(), err))`
- **为何安全**：标准库中 `Error::other(e)` 就是 `Error::new(ErrorKind::Other, e)`；kind、payload、`Display` 文本完全相同（behavior-traps 关注的错误文本未变）。
- **行为保持**：是（已应用 + 测试）。

### P3-2 `match` 取 Option 后 early return → `let … else` — `process.rs:222-225`、`256-259`
- **Before**
  ```rust
  let stdout = match self.child.stdout.take() {
      None => return Ok(()),
      Some(stdout) => stdout,
  };
  ```
- **After**
  ```rust
  let Some(stdout) = self.child.stdout.take() else {
      return Ok(());
  };
  ```
  第二处用 `self.child.stdout.as_mut()` 代替 `Some(ref mut stdout)`；借用在 `read` 后结束，随后写 `self.eof` 不冲突（编译通过证明）。
- **注意**：`close()` 中 `drop(stdout)` 必须保留——它是"关闭管道促使子进程退出"的显式时序（Drop 顺序属于 Rust ⚠ 项）。
- **行为保持**：是（已应用 + 测试）。

### P3-3（未应用）小噪声
- `process.rs:66-68` `From<io::Error>` 手写构造与 `CommandError::io` 重复，可写 `CommandError::io(ioerr)`。
- `process.rs:53-58` `write!(f, "\n{div}\n{msg}\n{div}", div = div, msg = msg.trim())` 中 `div = div` 冗余（2021+ 可内联捕获）。
均为一行级，收益太小。

## 刻意保留
- `parse_human_readable_size`：`checked_mul` + `ok_or_else`，已经是最小形态；溢出错误是契约。
- `StderrReader::read_to_end` 的两个 `expect`：表达不变量（只调用一次、线程不 panic），不是"该改成 `?` 的 unwrap"（改了就是 API 变化，examples B5）。
- `patterns_from_reader` 的 `for_byte_line` 闭包：返回 `Ok(true)` 是 bstr 的继续协议。
- 文档注释与 doctest：公共 API 文档，保留。

## 验证
- 本机 rustc 1.85 < MSRV 1.96：使用 `cargo test -p grep-cli --ignore-rust-version`（代码在 1.85 下可编译）。修改前后均 **22 passed（单元）+ 6 passed / 2 ignored（doctest）**。
- **局限**：box 上没有 rustfmt/clippy，格式按 `max_width = 79` 手工核对（无行超长），未经 rustfmt 机器验证；`hostname.rs:28` 在非 Linux 平台 cfg 下，Linux 上未被编译。
- diff：`applied.diff`（5 文件，+20/−33）。

## 置信度
高（语义等价由标准库定义保证）；格式中等（未跑 rustfmt）。
