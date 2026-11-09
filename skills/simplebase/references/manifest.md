# simplebase.json 清单

和 `examples/*/simplebase.json` 同一个格式，只取可以安全重放的部分。

```json
{
  "project": { "id": "myapp001", "name": "My App" },
  "databases": [
    { "name": "main", "data_model": "collection", "migrations": "migrations", "seed": "seed/dev.sql" }
  ],
  "collections": [
    { "database": "main", "name": "posts", "seed": [{ "id": "demo", "title": "hi" }] }
  ],
  "functions": [
    { "name": "hello", "file": "functions/hello.go", "description": "示例",
      "test": { "function_name": "Hello", "body": { "name": "sb" } } }
  ],
  "cron_jobs": [
    { "name": "daily", "schedule_kind": "cron", "cron_expr": "0 3 * * *",
      "func_file": "hello", "func_export": "Hello", "input_json": "{}", "enabled": true }
  ]
}
```

## 字段

| 字段 | 规则 |
| --- | --- |
| `project.id` | 8 位，字母、数字或 `-`。本地和云端用同一个 ID 建项目；脚本不负责建项目（需要超管，可用 `simplebase project create --id <ID> --name <名称>` 或控制台） |
| `databases[]` | **每个项目只能有一个用户数据库**（单文件模型）。声明多个时 validate 给出警告，建第二个库会返回 `database_already_exists`；需要文档又需要 SQL 时用一个 `collection` 库，迁移照样能跑 |
| `databases[].data_model` | `sql` 或 `collection`。**建议显式写**：不写时有 `migrations`/`seed` 且没有 `collections[]` 引用它就推断为 `sql`，否则 `collection`；已经用另一种类型建过库的环境会报 `data_model_mismatch`。examples 四个站点都显式写 `collection` |
| `databases[].migrations` | 目录，按文件名排序执行 `*.sql`；每个文件是一个事务 batch，最多 99 条语句 |
| `databases[].seed` | 单个 SQL 文件，只在 `--seed` 时执行，记账名为 `seed:<路径>` |
| `collections[]` | 只能挂在 collection 库下（显式写 `data_model: sql` 会被拒绝）；`seed` 文档必须有字符串 `id`。服务端建文档时会忽略这个 `id` 并生成 UUID，所以去重靠 `_migrations` 里的记账行 `seed-doc:<集合>/<id>`：已记账的跳过，不按文档 id 查询。业务代码不要依赖种子文档的 id，按内容字段查找 |
| `functions[]` | `name` 须字母开头、仅字母数字下划线、最长 63；`file` 是工作区内的相对路径；`test.function_name` 必须是导出函数；`description` 不写时保留远端值，写空串会把远端描述清空（与 cron 同语义） |
| `cron_jobs[]` | `schedule_kind` 取 `cron`（5 段 `cron_expr`）、`interval`（`interval_seconds` 60–2592000）或 `once`（RFC3339 `run_at`，过期则跳过）；`func_export` 必须是目标函数的导出函数；`input_json` 是 JSON 字符串，最大 8KB；`description` 不写时保留远端值，写空串会把远端描述清空 |

会被忽略并给出警告的字段：`kv`、`objects`（数据）、`api_keys`（会产出密钥）、`agents`、`requires`、`trigger_once`（有副作用）。

路径必须在工作区内，且不能引用 `.env*`。validate 会扫描被引用的文件，发现疑似密钥（私钥头、`sk-`、`AKID`、`AKIA`、`ghp_`、SimpleBase 自身的 `sb_live_`）就判为失败。函数没配 `test` 时给出警告。

## 重放顺序与判定

校验 → 库 → 迁移（+种子）→ 集合 → 函数 → 定时任务。每项输出 `create`、`update`、`apply` 或 `skip`。

- **库**：优先使用 lock 里的 ID，否则按名称查找；远端 `data_model` 不一致时报错。
- **迁移**：记账表 `_migrations(name, applied_at)`，与 examples 共用；已应用的文件跳过。
- **集合种子**（仅 `--seed`）：同一张 `_migrations` 记 `seed-doc:<集合>/<id>`，重复 apply 不会重复建文档。
- **函数**：生效版本源码的 sha256 与本地一致时跳过；否则复用源码相同的最新版本，或新建版本，再试跑（`test`），最后生效。新函数用 `function create --activate false` 建出未生效的 v1，试跑通过后才 `activate`，所以试跑失败的函数不会上线。
- **cron**：按 name 匹配；调度、目标、input、enabled、description 任一不同就带全量字段更新。目标不是云函数的同名任务会报错。
- **description 三态**（函数与 cron 一致）：不写 = 保留远端值；写空串 = 显式清空；写非空 = 不一致就更新。

## 对账与提示

`internal/sbcli/testdata/dev/{basic,invalid}` 的 `expect.json` 同时被 Go 与 Python 测试读取，**错误与警告的文本、顺序、条数都严格比对**。改提示文案时要两端一起改，否则测试直接失败。
- 远端多出来的资源不会被删除。

## 生成的文件

| 文件 | 内容 | 提交 git |
| --- | --- | --- |
| `.simplebase/lock.json` | 按目标分区：远端是 `URL#project`，本地统一为 `local#project`（不带端口，避免每人一份）；含库 ID、引擎、迁移 sha、函数版本、cron ID | 可以（无密钥）；团队共用时建议提交 |
| `.simplebase/env.<label>` | URL、project ID、`SIMPLEBASE_DATABASE_ID_<NAME>`；只有单个库时才额外写不带后缀的 `SIMPLEBASE_DATABASE_ID`。`label`：URL 主机是 `127.0.0.1`/`localhost`/`::1`/`0.0.0.0` 时固定为 `local`，否则取主机名（小写，非字母数字换成 `-`） | 不提交（init 已加入 .gitignore） |
| `.env`、`.env.cloud` | `SIMPLEBASE_URL`、`SIMPLEBASE_API_KEY`、`SIMPLEBASE_PROJECT_ID` | 不提交 |

`dev apply` 会在 `--dir` 下写 `.simplebase/`；对仓库里的样例（如 `examples/<case>`）验证时先复制到临时目录，避免留下未跟踪文件。

**本地与远端的判定**只看 URL 主机：上面四个主机名算本地（apply 不需要 `--yes`，lock 分区为 `local#<project>`），其他一律算远端。想在本机模拟远端（例如起第二个实例）时用 `http://localhost.:<端口>` 这类非字面量主机。

凭据读取顺序：`--env-file` 指定的文件 > 进程环境 > 工作区 `.env`。Key 只取自优先级最高、且设置了 Key 的那一个来源（不会把云端 URL 和进程里残留的本地 Key 拼在一起）；同一来源里 `SIMPLEBASE_TOKEN` 优先于 `SIMPLEBASE_API_KEY`，与 CLI 一致。脚本把它作为 CLI 的 token，通过子进程环境传入。

## 两种入口，同一份行为

| 入口 | 适用 | 凭据来源 |
| --- | --- | --- |
| `scripts/sbdev.py` | 没有新版 `simplebase` 二进制，或需要 `init` / `doctor` / 自动读取工作区 `.env` | `--env-file`、进程环境、工作区 `.env` |
| `simplebase dev validate\|plan\|apply\|status` | 二进制已带 `dev` 子命令（planv6.4 P0b）；CI 与 examples 初始化使用 | flag、`SIMPLEBASE_*` 环境变量、`config.json`（不读 `.env`）；`validate` 与 `status` 不需要连接配置 |

两者共用同一份清单、lock 格式和重放语义，并用同一组 fixture（`internal/sbcli/testdata/dev`）对账。`simplebase dev` 额外支持 `--manifest <文件>`：清单取自该文件，其中引用的迁移与函数仍相对 `--dir`。它读取宿主机文件，所以助手模式（`SIMPLEBASE_AGENT_RUN=1`）下一律返回 `file_not_allowed`。

## 退出码

`0` 成功；`1` 校验或用法错误（包括 `migration_modified`）；`2` 远端错误（打印 `error.code`）；`3` 远端 apply 需要 `--yes`。
