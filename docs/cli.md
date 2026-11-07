# simplebase CLI

`simplebase` 把控制台 REST API 收成一组命令。云助手在 `agent.skills_cli` 打开时通过它写用户资源，不直接碰存储。

## 开关

代码默认 `agent.skills_cli: true`。环境变量覆盖 YAML：

```bash
SIMPLEBASE_AGENT_SKILLS_CLI=true
```

关闭：

```yaml
agent:
  skills_cli: false
```

或 `SIMPLEBASE_AGENT_SKILLS_CLI=false`。委托有效期 `agent.delegation_ttl`（默认 15 分钟），确认等待 `agent.confirm_timeout`（默认 2 分钟）。

## 人直接使用

```bash
simplebase --endpoint http://127.0.0.1:8080 --token "$SIMPLEBASE_TOKEN" --project "$SIMPLEBASE_PROJECT_ID" database list
```

也可以把 `endpoint`、`token`、`project_id` 写进 `$XDG_CONFIG_HOME/simplebase/config.json`（权限必须是 `0600`）。优先级是命令行 flag、环境变量、配置文件。

环境变量里的 token 读 `SIMPLEBASE_TOKEN`，也认 SDK 使用的 `SIMPLEBASE_API_KEY`；两者都设置时 `SIMPLEBASE_TOKEN` 优先，空值视为未设置。这样同一份 `.env` 既能给 SDK 用，也能给 CLI 用。

成功时 stdout 一行 `{"ok":true,"data":...}`。失败时 stderr 一行 `{"ok":false,"error":{"code":...}}`，退出码 2。用法错误退出码 1，配置错误退出码 4。

几个容易踩的返回形状：

- `function invoke`：`data` 就是函数返回值。`function test`（试跑）多一层信封，`data` 是 `{ok,status_code,duration_ms,version,function_name,data,error}`，函数返回值在 `data.data`。
- `cron runs`：`data.runs[].status` 为 `running` / `completed` / `failed` / `canceled`，成功是 `completed`。
- `settings get`：`data.settings[]` 的字段为 `key` / `value_json` / `updated_at`。
- `schema rows`：`data.columns` 是列对象数组（`name` / `type` / `nullable`），不是列名字符串；列名字符串数组是 `sql query` 的形状。

## 数据导入导出与迁移检查

```bash
simplebase document import --db "$DATABASE" --collection users --format ndjson --file ./users.ndjson
simplebase document import --db "$DATABASE" --collection users --format csv --dry-run < ./users.csv
simplebase document export --db "$DATABASE" --collection users --format csv --file ./users.csv
simplebase document export --db "$DATABASE" --collection users --format ndjson --filter 'status="open"'
simplebase migrate status   # admin 项目
simplebase migrate plan
```

导入失败整体回滚；`--dry-run` 只校验。未指定导出 `--file` 时 stdout 是原始 CSV/NDJSON，不是 CLI JSON 信封。迁移命令只读，不执行 DDL。

## 工作区重放：`dev validate|plan|apply|status`

把本地工作区（`simplebase.json`、迁移、云函数、定时任务、集合声明）幂等地重放到任意一套 SimpleBase，教程见[本地开发、云端重放](/docs/getting-started/local-cloud-collab)。

```bash
simplebase dev validate [--dir DIR] [--manifest FILE]   # 纯离线校验，不连远端
simplebase dev plan     [--dir DIR] [--manifest FILE] [--seed]
simplebase dev apply    [--dir DIR] [--manifest FILE] [--seed] [--yes]
simplebase dev status   [--dir DIR]                     # 只读 .simplebase/lock.json
```

- `validate`、`status` 只读工作区文件，**不需要** `SIMPLEBASE_URL` / token / project，适合只有二进制、没有凭据的 CI；`validate` 失败返回 `workspace_invalid` 与 `data.errors`。
- `plan` 只读远端；`apply` 对本地地址（`127.0.0.1` / `localhost` / `::1` / `0.0.0.0`）直接执行，对其它地址必须带 `--yes`，否则只输出计划并以**退出码 3** 退出，不做任何写入。
- `--seed` 时集合种子文档按 `_migrations` 里的 `seed-doc:<集合>/<id>` 记账去重（服务端建文档会忽略清单里的 `id`），重复执行不会重复建文档。
- 每个项目只能有一个用户数据库；清单声明多个库时 `validate` 给出警告。
- 管理员可用 `simplebase project create --id <8位ID> --name <名称>` 在多个实例上建同 ID 项目，供同一工作区重放。
- 输出 `data.actions[]`（`verb` 为 `create` / `update` / `apply` / `skip`）、`changes`、`skips`、`warnings`；`apply` 另有 `env_hint`（`.simplebase/env.<label>`，不含密钥）。
- lock 按目标分区：远端是 `<URL>#<project>`，本地统一为 `local#<project>`（不带端口，多人共用一个分区；库 ID 失效时回退按名称匹配）。
- `plan` / `apply` 会读一次 `project list` 记录项目引擎，与 lock 里其它环境的记录不一致时给出警告（迁移只按一种方言写）。
- 退出码：`0` 成功，`1` 校验或用法错误（含 `migration_modified`），`2` 远端错误（含 `project_not_found`），`3` 需要 `--yes`，`4` 配置缺失。
- 命令读取宿主机上的工作区文件，所以助手模式（`SIMPLEBASE_AGENT_RUN=1`）下返回 `file_not_allowed`，不属于云助手 skill。
- 另有 `function create --activate false`：只创建未生效的首个版本，试跑后再 `function activate`。

## 助手模式（SIMPLEBASE_AGENT_RUN=1）

助手运行时由云助手宿主设置 `SIMPLEBASE_AGENT_RUN=1`，与人类使用有三个差异：

### 1. 文件内容内联，不读宿主机文件

`--file` 只接受 `-`（stdin），宿主机路径一律返回 `file_not_allowed`。文件内容作为工具参数 `stdin` 传入，上限 256KiB：

```
simplebase function create --name hello --file -        ← 源码放 stdin
simplebase sql batch --database shop --file -           ← JSON 放 stdin（或改用 --statement）
simplebase object upload --key readme --file -          ← 内容放 stdin
simplebase database create --name shop --init-sql -     ← SQL 放 stdin
```

也可以显式内联：`--source-b64`（function create）、`--statement`（sql exec/batch）、`--content-base64`（object upload）。`document export` 在 Agent 模式一律返回 `agent_raw_export_disabled`，不会把未脱敏原始数据交给模型。

### 2. 委托令牌只能访问白名单路由

助手拿到的委托令牌只在带 `delegable` 标记的路由上有效（数据库、KV、S3、云函数、定时任务、沙盒、只读日志、配额/审计），其余路由（助手自身、用户、凭据、设置）一律 403。可委托集合由 `TestDelegation_RouteTable` 锁定。

### 3. 确认与取消只认发起人

破坏性命令（DROP/DELETE 类、`cron trigger`、`object delete` 等）需要界面确认；确认与取消只对发起人本人或 `project:admin` 生效（`sys_agent_runs.initiator_id`）。

## 助手能力边界（planv6.0）

- `users` skill 已下线：助手不做用户管理；`user`/`apikey`/`settings` 命令保留给人类操作者，不属于任何 skill。
- `project` skill 只保留 `quota get` 与 `audit list`。
- 内置助手默认 `writable=false`：写命令返回 `write_not_allowed`，且在确认弹窗之前就失败。在「云助手 → 编辑助手 → 允许写操作」打开后才可写，破坏性命令仍需确认。

## 本地 gocloudagent 的 shell 环境

本地 `gocloudagent` 二进制的 shell 工具会剥离 `SIMPLEBASE_*`、`GOCLOUDAGENT_*` 以及名字含 `TOKEN` / `SECRET` / `PASSWORD` / `API_KEY` / `ACCESS_KEY` / `SESSION` 的环境变量。shell 里再跑 `simplebase` 时读不到这些变量，会返回 `config_missing`（退出码 4）。要让它操作同一套资源，需事先写好 `$XDG_CONFIG_HOME/simplebase/config.json`（权限 `0600`），并且不要让模型把该文件内容打印出来。服务端助手使用专用 `simplebase` 工具，不受这条限制。
