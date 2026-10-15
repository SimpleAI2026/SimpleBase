# 云函数：创建、版本、试跑与调用

源码是单文件 `package main`。`Greet` 一个入参、一个返回值。`function create` 会生成版本 1 并立即生效。`function versions create` 追加版本 2 但不生效，需要 `activate` 之后 `invoke` 才会跑新源码。`test` 可以在激活前跑指定版本。

文件头的 `//go:build ignore` 只是为了不让 `go test ./...` 把示例源码当成仓库里的包；解释器把它当普通注释。

## 运行

```bash
export CASE=06-functions
./examples/capabilities/06-functions/run.sh
```

## 命令

```bash
simplebase function create --name "greet$RUN_ID" --file greet_v1.go --description "capability example"
simplebase function get --name "greet$RUN_ID"
simplebase function invoke --name "greet$RUN_ID" --export Greet --input '{"name":"simplebase"}'
simplebase function versions create --name "greet$RUN_ID" --file greet_v2.go --note "v2"
simplebase function test --name "greet$RUN_ID" --version 2 --export Greet --input '{"name":"simplebase"}'
simplebase function activate --name "greet$RUN_ID" --version 2
simplebase function invoke --name "greet$RUN_ID" --export Greet --input '{"name":"simplebase"}'
```

助手模式不能读宿主机路径，要改成 `--file -` 并把源码放进 stdin。人类 CLI 可以直接传文件。
