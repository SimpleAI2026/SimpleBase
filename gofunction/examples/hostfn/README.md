# hostfn

`BuildProgram` 固定使用 `importer.GlobalRegistry`。在 `Run` 之前调用 `importer.RegisterPackage`，脚本即可 `import` 该 path。

默认不允许重复注册同一 path。

```bash
go run ./gofunction/examples/hostfn
```

期望输出：`host.Add(20, 22) = 42`
