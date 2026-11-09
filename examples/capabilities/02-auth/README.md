# 用户与 API Key

人类操作者通过 CLI 创建普通用户、签发只读 API Key、用该 Key 读库、确认它不能写，然后吊销 Key 与删除用户。

云助手 skill 不开放 `user` / `apikey`。口令只从 stdin 读入，脚本会生成随机口令并且不打印；API Key 明文只留在进程里，结果记录会打码。

新建用户的 `must_change_password` 为 true，因此不能直接用初始口令登录。本示例验证的是签发、权限边界和吊销，不是该用户的登录。

## 运行

需要超管访问令牌，以及 `01-project` 写下的 `SIMPLEBASE_PROJECT_ID`。

```bash
export CASE=02-auth
./examples/capabilities/02-auth/run.sh
```

## 命令

```bash
printf '%s' "$PASSWORD" | simplebase user create --username "exu$RUN_ID" --role user --display-name "Capability example"
simplebase user get --id "$USER_ID"
simplebase user list --limit 20
simplebase apikey create --permissions database:read
simplebase apikey list
# 用新 Key 作为 SIMPLEBASE_TOKEN：database list 成功，database create 被拒绝
simplebase apikey revoke --id "$KEY_ID"
simplebase user delete --id "$USER_ID"
```
