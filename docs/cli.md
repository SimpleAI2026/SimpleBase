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

成功时 stdout 一行 `{"ok":true,"data":...}`。失败时 stderr 一行 `{"ok":false,"error":{"code":...}}`，退出码 2。用法错误退出码 1，配置错误退出码 4。

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

也可以显式内联：`--source-b64`（function create）、`--statement`（sql exec/batch）、`--content-base64`（object upload）。

### 2. 委托令牌只能访问白名单路由

助手拿到的委托令牌只在带 `delegable` 标记的路由上有效（数据库、KV、S3、云函数、定时任务、沙盒、只读日志、配额/审计），其余路由（助手自身、用户、凭据、设置）一律 403。可委托集合由 `TestDelegation_RouteTable` 锁定。

### 3. 确认与取消只认发起人

破坏性命令（DROP/DELETE 类、`cron trigger`、`object delete` 等）需要界面确认；确认与取消只对发起人本人或 `project:admin` 生效（`sys_agent_runs.initiator_id`）。

## 助手能力边界（planv6.0）

- `users` skill 已下线：助手不做用户管理；`user`/`apikey`/`settings` 命令保留给人类操作者，不属于任何 skill。
- `project` skill 只保留 `quota get` 与 `audit list`。
