# funclist

`ParseFuncList(src, exportedAll)`：

- `false`：仅包级导出函数（跳过方法、`init`、未导出函数）
- `true`：所有 `func` 声明（含方法与未导出函数）

不做类型检查，也不执行。

```bash
go run ./gofunction/examples/funclist
```

期望：`exportedOnly = [Exported]`，`exportedAll` 含 `Exported`、`hidden`、`Method`。
