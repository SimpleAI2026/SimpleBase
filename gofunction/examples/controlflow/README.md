# controlflow

解释执行包含 `for`、`switch`、`if`、`defer` 的脚本（语言子集，不是完整 Go 编译器）。

```bash
go run ./gofunction/examples/controlflow
```

`classify(10)`：1..10 求和为 55，走 `medium` 且偶数，named return 上的 `defer` 再追加 `/ok`。期望输出：`medium/even:55/ok`
