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
simplebase --endpoint http://127.0.0.1:8080 --token "$SIMPLEBASE_TOKEN" --project dev-shop database list
```

也可以把 `endpoint`、`token`、`project_id` 写进 `$XDG_CONFIG_HOME/simplebase/config.json`（权限必须是 `0600`）。优先级是命令行 flag、环境变量、配置文件。

成功时 stdout 一行 `{"ok":true,"data":...}`。失败时 stderr 一行 `{"ok":false,"error":{"code":...}}`，退出码 2。用法错误退出码 1，配置错误退出码 4。
