# gofunction
<!-- allow: function -->
<!-- readonly: function list,function get,function test -->

命令：`function list|get|create|update|delete|versions|activate|test|invoke`。云函数由进程内 Go 解释器执行，按名称加版本管理；调用只跑「生效版本」。

## 源码约定

- 单文件、`package main`，每个**大写**顶层函数就是一个可调用导出，必须恰好 1 个入参、1 个返回值，如 `func Hello(req Request) Response`。入参/返回值用 struct（字段须大写并带 `json` tag）、`map[string]interface{}` 或基础类型；小写函数不可调用。
- 只能 import 已注册包：`fmt strings math time regexp strconv errors bytes io io/ioutil sync/atomic net/http encoding/json encoding/base64 encoding/binary github.com/json-iterator/go`，且只含常用函数（如 `strings` 无 `Builder`，`fmt` 无 `Fprintf`，无 `os`/`sort`/`sync`/`context`）。引用未注册符号会编译失败。
- 支持常用语言特性（struct、slice/map、闭包、`defer`、`panic/recover`、`go` 与 channel），但无 cgo、无 `go.mod`；单次执行默认 10 秒超时；`print`/`println` 输出不会返回，调试请返回结构化结果。
- 源码上限 256KiB，一项目最多 100 个函数、每个函数最多 50 个版本。名称须字母开头、字母数字下划线、最长 63。

## create / versions（源码走 stdin）

助手模式不能读宿主机文件，源码放入工具参数 `stdin`，argv 用 `--file -`：

```
simplebase function create --name hello --file - --description "示例"   ← 源码放 stdin，创建后自动生效
simplebase function versions create --name hello --file - --note "fix"   ← 新版本默认不生效
simplebase function activate --name hello --version 2
```

`versions create` 只追加版本，必须再 `activate` 才会被 `invoke` 与定时任务使用。

## 查看与调试

```
simplebase function list
simplebase function get --name hello
simplebase function versions list --name hello
simplebase function versions get --name hello --version 2
simplebase function test --name hello --version 2 --export Hello --input '{"name":"a"}'
simplebase function invoke --name hello --export Hello --input '{"name":"a"}'
```

- `test` 在调试台跑指定版本（可测未生效版本），`--version`、`--export` 必填；`invoke` 只跑生效版本。两者 `--input` 为 JSON，缺省 `{}`；`test` 单条命令最长等待 120 秒。
- 返回值的位置不同：`invoke` 的 `data` 就是函数返回值；`test` 的 `data` 是试跑信封 `{ok,status_code,duration_ms,version,function_name,data,error}`，函数返回值在 `data.data`。
- `function update --name hello --description "..."` 只改描述。
- `function delete --name hello` 需要界面确认；headless 返回 `confirmation_unavailable`，不要重试。

## 常见错误码

- `gofunction_signature_invalid`：导出函数签名不合约定，按 message 里的函数名修改。
- `gofunction_compile_error`：语法错误或引用了未注册符号，按 message 修正。
- `gofunction_runtime_error`/`request_timeout`：运行期 panic 或超时，精简逻辑。
- `gofunction_not_found`（核对 `--name`）、`function_not_found`（`--export` 不存在或未导出）、`no_active_version`（先 `activate`）。
- `gofunction_already_exists`、`gofunction_limit_exceeded`、`version_limit_exceeded`、`too_many_go_runs`（并发满，稍后重试一次）、`system_project_protected`（系统项目不支持云函数）。
- `file_not_allowed`：`--file` 传了路径，改用 `--file -`。
