# pool

`ExecutorPool` 复用 `Executor` 实例。注意：

- `Execute(functionName, script)` **不能传函数参数**（要传参请用 `gofunction.Run`）
- 每次 `Execute` 都会重新 `BuildProgram`，池复用的是执行器对象而不是编译缓存

```bash
go run ./gofunction/examples/pool
```

`Close` 之后 `GetStats` 的 idle 应 ≥ 1，第二次 `GetExecutor` 仍能执行同一脚本。
