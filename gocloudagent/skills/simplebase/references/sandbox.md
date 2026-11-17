# sandbox
<!-- allow: sandbox -->
<!-- readonly: sandbox capabilities,sandbox list,sandbox get -->

命令：`sandbox capabilities|list|get|create|delete|connect|pause`（项目沙盒 API，不是线程内的 `sandbox_*` 工具）。沙盒是后端上的进程隔离环境，没有 SimpleBase 凭据，访问不到项目数据库、对象存储与系统库；E2B SDK 与线程内工具通过沙盒内的 envd 执行命令与文件读写。

沙盒未启用时路由不存在，命令返回 `not_found`/`sandbox_unavailable`，不要反复尝试，如实告诉用户。`start`、`stop`、`exec`、`update`、`run`、`files` 子命令已删除，需要在沙盒里执行命令、读写文件时，请使用线程内的 `sandbox_exec`、`sandbox_shell`、`sandbox_read_file`、`sandbox_write_file` 工具，工作目录 `/workspace`。

## capabilities

```
simplebase sandbox capabilities
```

无参数、只读，直接可执行。返回实例支持的能力与默认值（如模板、超时上限、网络档位）；创建前想确认可选值、或 `create` 报 `unsupported_feature` 时，先查这个命令。返回里没有列出的能力就是当前实例不支持的，不要假设。

## 生命周期

```
simplebase sandbox list
simplebase sandbox list --state paused --limit 20
simplebase sandbox get --id <沙盒ID>
simplebase sandbox create --name dev --template base --timeout 600
simplebase sandbox delete --id <沙盒ID>   ← 需要确认
simplebase sandbox connect --id <沙盒ID> --timeout 300
simplebase sandbox pause --id <沙盒ID>
```

`list` 的 `--state` 可选 `running`/`paused`，`--limit` 取 1–100（省略用默认值，越界报 `invalid_request`）。`create` 的 flag 是 `--name`、`--template`（省略时用默认）、`--timeout`（秒，整数，省略用实例默认），不能指定规格。`connect` 恢复已暂停的沙盒或延长运行中沙盒的存活时间；`pause` 停止沙盒但保留工作区文件（不保存进程与内存）。`sandbox delete` 需要界面确认；headless 返回 `confirmation_unavailable`，不要重试。

## 常见错误码

`sandbox_not_found`（核对 `--id`）、`sandbox_name_conflict`（换名）、`sandbox_limit_exceeded`（项目沙盒数达上限，先删旧的）、`sandbox_busy`、`sandbox_gone`（已过期，重新创建）、`sandbox_backend_error`、`sandbox_unavailable`（沙盒系统不可用）、`unsupported_feature`（实例配置不支持）、`forbidden`（权限不足或系统项目不支持沙盒）。
