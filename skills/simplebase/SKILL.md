---
name: simplebase
description: 在本地用 SimpleBase 开发后端（Go 云函数、SQL 迁移、集合、定时任务），并把同一份工作区幂等重放到云端 SimpleBase。用户要搭 SimpleBase 项目、写或改云函数、加表或迁移、配置 cron、本地联调、把本地成果发布或同步到云端、在环境之间切换 SIMPLEBASE_URL，或排查 simplebase CLI 报错时使用，即使没有提到 skill 或 CLI。不用于在控制台云助手里操作已在运行的项目，也不用于用户、API Key、实例设置管理。
---

# SimpleBase 本地开发与云端重放

工作区就是产物：`simplebase.json` 清单，加上 `migrations/*.sql`、`functions/*.go`，用 git 管理。`scripts/sbdev.py` 读取工作区，通过本机 `simplebase` CLI 把它重放到某个环境，先查远端状态再写，所以中断后重跑就能接着做。本地和云端用同一个脚本，只是 `.env` 不同。

## 工作流

1. **准备**：`python3 scripts/sbdev.py init -C <dir> --project-id <8位ID>`。本地和云端要用同一个 project ID，业务代码才能只换 URL。脚本不建项目：请用户在两个环境的控制台「新建项目」里填同一个「项目 ID」（可选字段，留空会随机生成），或由管理员执行 `simplebase project create --id <ID> --name <名称>`，并签发 Key。然后 `cp .env.example .env`，让用户自己填 Key。
2. **自检**：`sbdev.py doctor -C <dir>`。全部 `[ok]` 后再继续。
3. **开发循环**：改代码 → `validate` → `plan` → `apply` → 用 CLI 验证（`function invoke`、`sql query`）。本地 apply 不需要确认。
4. **发布到云端**：`plan --env-file .env.cloud`，把变更清单给用户看，用户明确同意后才执行 `apply --env-file .env.cloud --yes`。远端 apply 不带 `--yes` 时只打印计划，并以退出码 3 退出。plan 里出现 `update function` 或迁移含 DROP/DELETE 警告时，要单独向用户指出。
5. **切换环境**：apply 会写 `.simplebase/env.<label>`，里面有 URL、project ID、库 ID（不含密钥）。业务侧改 `SIMPLEBASE_URL` 和 `SIMPLEBASE_API_KEY`；按库名解析库 ID，或使用 env 文件里的 ID。

脚本路径相对于本 skill 目录。命令都接受 `-C <工作区>`，远端相关命令还接受 `--env-file`、`--url`、`--project`、`--bin`。凭据优先级：`--env-file` > 进程环境 > 工作区 `.env`，Key 只取自优先级最高的那个来源。

二进制自带 `simplebase dev validate|plan|apply|status` 时，行为与 `sbdev.py` 一致，CI 里优先用它；`init` 和 `doctor` 只有 `sbdev.py` 才有。`simplebase dev` 不读 `.env`（凭据要来自环境变量、flag 或 `config.json`），`validate` 与 `status` 不需要任何连接配置，可以直接 `simplebase dev validate --dir .`。若退出码 1 且报 `unknown command dev validate`，说明二进制太旧，改用 `sbdev.py`。两者的区别见 `references/manifest.md`。验证脚本自身：`python3 -m unittest discover -s scripts -p 'test_sbdev.py'`（在本 skill 目录下，用假 CLI，不连真实服务）。

## 写代码时的规则

- **迁移只追加**：已应用的文件不能改，改了 apply 会报 `migration_modified`。要改表结构就新增 `00N_xxx.sql`。迁移里不写 `PRIMARY KEY`/`UNIQUE`/`CREATE INDEX`/`CREATE SEQUENCE`，因为 DuckLake 不支持；写 `CREATE TABLE IF NOT EXISTS`。语句按 `;` 拆分时会跳过引号和注释里的分号，所以字面量里可以写 `;`。细节见 `references/database.md`。
- **云函数**：单文件 `package main`，大写顶层函数作为导出，恰好 1 个入参、1 个返回值，只能 import 已注册的包。每个函数都配 `test`（没配时 validate 会警告，apply 不经试跑直接生效）。见 `references/gofunction.md`。
- **清单**：字段和忽略项见 `references/manifest.md`。`kv`、`objects`、`api_keys`、`trigger_once` 不进入重放。函数与 cron 的 `description` 都是三态：不写保留远端值，写空串显式清空。
- 先读现有代码和清单再改；改完至少跑一次 `validate`。

## 边界

- **密钥**：不要让用户把 Key 贴进对话，也不要打印或 `cat` `.env*` 和 `~/.config/simplebase/config.json`。Key 只放在 `.env*`（gitignore）里，由用户在控制台「设置 → 连接」签发。发布 Key 需要 `database:admin`（隐含 `database:write`、`database:read`）；业务运行时的 Key 只给 `database:read`/`database:write`，不要复用发布 Key。
- **单库**：每个项目只能有一个用户数据库，清单里只声明一个库。删除这个库会清空它的全部表与集合（KV、对象文件、备份保留），不可恢复，必须先提醒用户。
- **破坏性操作不自动做**：重放不删除远端资源，不触发 cron。删库、删函数、删任务、`cron trigger`、`--seed` 写云端，都要用户明确同意后，再单独执行。
- **数据不搬**：默认只同步结构和代码。`--seed` 只用于开发夹具，不要对生产执行。
- **失败处理**：根据 `error.code` 修参数或代码。同一条命令原样重试不会有不同结果；`lease_*`、`too_many_go_runs` 这类错误可以稍后重试一次。错误码速查见 `references/cli.md`。
- 控制台云助手使用的是另一份内置 skill（`gocloudagent/skills/simplebase`），本 skill 不适用于那里。

## 参考

| 文件 | 何时读 |
| --- | --- |
| `references/manifest.md` | 写或改 `simplebase.json`（含 cron `description` 的空值语义）；lock 与 env 文件的含义 |
| `references/gofunction.md` | 写云函数 |
| `references/database.md` | 写迁移、SQL、集合 |
| `references/cli.md` | 直接调用 `simplebase` CLI，或遇到错误码 |
