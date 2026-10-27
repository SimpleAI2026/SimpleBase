---
name: simplebase
description: 通过 `simplebase` CLI 工具操作当前 SimpleBase 项目的资源：SQL/集合数据库、键值 KV、对象存储 S3、云函数（Go）、定时任务 cron、沙盒、日志检索、配额与审计。只要用户在 SimpleBase 助手里想查询或修改项目数据、建库建表、读写键值、上传对象、部署或调用云函数、配置定时任务、跑沙盒命令、查日志或查配额，即使没有说出“CLI”或具体命令名，也应使用本 skill。不用于用户、API Key、实例设置管理。
---

# SimpleBase
<!-- allow: -->

本 skill 教你用 `simplebase` 工具代用户操作**当前项目**。工具是宿主内置的 CLI，不是 shell：参数和结果都是结构化的，宿主会按已加载模块放行命令、做权限和确认检查。

## 调用方式

- `argv` 是字符串数组，不含二进制名，也不拼 shell 字符串。宿主不经过 shell 解析，引号和管道都不会生效。
- 文件内容、函数源码、批量 JSON 都放进工具参数 `stdin`，argv 里写 `--file -`。助手不能读宿主机文件，传路径会返回 `file_not_allowed`。
- 数据库用 `--database <库ID>`，不是库名。先 `database list` 取 `id`，因为库名可以重复或改动，ID 才是稳定标识。

## 结果协议

成功：`{"ok":true,"data":...}`；失败：`{"ok":false,"error":{"code":...,"message":...}}`。

按 `error.code` 决定下一步。同一条失败命令原样重试不会有不同结果，应先改参数或如实告知用户。没有拿到的数据不要编造；结果里出现密钥或 token 也不要转述，它们属于用户的凭据。

## 边界

- 所有命令只作用于当前项目，不能跨项目。
- 破坏性命令（删除、`DROP`、`kv DEL`、`cron trigger` 等）需要用户在界面确认。定时任务等无人值守运行时会返回 `confirmation_unavailable`，重试也没有人能确认，所以直接报告。
- 只读助手执行写命令返回 `write_not_allowed`，且不会先弹确认，直接失败。这是用户授予的权限范围，如实说明并告诉用户可在「编辑助手 → 允许写操作」中开启，不要换命令绕过。
- 用户、API Key、设置管理不通过助手开放，被要求时请用户去控制台操作。

## 模块与参考文档

每个模块的命令、必填参数、示例和错误码在 `references/` 下。宿主只放行**已加载**模块的命令，未加载时返回 `skill_not_loaded`。加载哪些模块由**助手本身**决定：专用助手自动注入自己模块的文档，通用助手不注入任何模块文档。用户输入里的 `@` 只用于点名助手，不用于选择模块；缺哪个模块的能力，请让用户新建或切换到对应助手，不要猜测调用。

| 模块 | 对应助手 | 放行的命令 | 参考文档 |
| --- | --- | --- | --- |
| database | Database | `database` `sql` `schema` `collection` `document` | `references/database.md` |
| kv | 键值助手 | `kv` | `references/kv.md` |
| s3 | S3 | `object` | `references/s3.md` |
| gofunction | 云函数助手 | `function` | `references/gofunction.md` |
| cron | 定时任务助手 | `cron` | `references/cron.md` |
| sandbox | 沙盒助手 | `sandbox` | `references/sandbox.md` |
| logs | Logs | `log` | `references/logs.md` |
| project | 无（仅前缀白名单） | `quota` `audit` | `references/project.md` |

跨模块任务（如定时任务调用云函数）需同时加载相关模块；通用助手不加载子模块，需要跨模块写入时请切换到相应专用助手。维护这些文档的格式约定见 `references/authoring.md`，执行任务时不需要读它。
