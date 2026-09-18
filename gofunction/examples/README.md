# gofunction examples

每个子目录都是独立的 `package main`，在仓库根（模块 `github.com/linkxzhou/SimpleBase`）执行：

```bash
go run ./gofunction/examples/hello
```

或一次跑完全部：

```bash
for d in gofunction/examples/*/; do echo "== $d"; go run ./$d; done
```

| 目录 | 内容 |
| --- | --- |
| [hello](hello) | `gofunction.Run` |
| [stdlib](stdlib) | `fmt` / `strings` / `encoding/json` |
| [globals](globals) | `SetGlobalValue` / `GetGlobalValue` |
| [funclist](funclist) | `ParseFuncList` |
| [hostfn](hostfn) | `importer.RegisterPackage` |
| [pool](pool) | `ExecutorPool` |
| [controlflow](controlflow) | if / for / switch / defer |
| [http](http) | `httptest` + 脚本 `http.Get` |

用法说明见上一级 [README.md](../README.md)。
