# globals

`BuildProgram` 之后用 `SetGlobalValue` / `GetGlobalValue` 读写包级变量。SSA 全局是 `*T` 单元，这两个方法操作的是单元里的值。

```bash
go run ./gofunction/examples/globals
```

期望：

```
N after init = 1
N after SetGlobalValue = 9
current() = 9
```
