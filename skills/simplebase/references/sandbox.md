# sandbox
<!-- allow: sandbox -->
<!-- readonly: sandbox list,sandbox get,sandbox run -->

命令：`sandbox list|get|create|update|delete|run`（项目沙盒 API，不是线程内的 `sandbox_*` 工具）。沙盒是云上隔离微虚机，没有 SimpleBase 凭据，访问不到项目数据库、对象存储与系统库。

沙盒未启用时路由不存在，命令返回 `not_found`/`sandbox_unavailable`，不要反复尝试，如实告诉用户。`capabilities`、`start`、`stop`、`exec`、`files` 子命令不对助手开放（返回 `forbidden`）；需要在沙盒里执行命令、读写文件时，用线程内的 `sandbox_exec`、`sandbox_shell`、`sandbox_read_file`、`sandbox_write_file` 工具，工作目录 `/workspace`。

## 一次性执行

```
simplebase sandbox run --command "go version"
```

必填：`--command`；`--image` 可选，省略时用实例默认镜像，不要猜镜像名。执行完即回收，超时 120 秒，stdout/stderr 超出上限会被截断。`run` 不需要界面确认，但属于写操作，只读助手会被后端拒绝为 `forbidden`。

## 生命周期

```
simplebase sandbox list
simplebase sandbox get --id <沙盒ID>
simplebase sandbox create --name dev
simplebase sandbox update --id <沙盒ID> --name dev2
simplebase sandbox delete --id <沙盒ID>   ← 需要确认
```

`sandbox delete` 需要界面确认；headless 返回 `confirmation_unavailable`，不要重试。

## 常见错误码

`sandbox_not_found`（核对 `--id`）、`sandbox_name_conflict`（换名）、`sandbox_limit_exceeded`（项目沙盒数达上限，先删旧的）、`sandbox_invalid_spec`（参数非法，去掉 `--image` 用默认镜像）、`sandbox_busy`、`sandbox_gone`（已过期，重新创建）、`sandbox_backend_error`、`system_project_protected`（系统项目不支持沙盒）。
