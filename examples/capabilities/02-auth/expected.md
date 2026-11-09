# 预期结果

| 步骤 | 通过条件 |
|---|---|
| `user create` | `ok=true`，`data.id` 非空，`data.username` 为小写用户名，响应里没有口令 |
| `user get` / `user list` | 能按 id 取回该用户，列表里看得到 |
| `apikey create --permissions database:read` | `ok=true`，`data.secret` 非空（不写入结果文件），`data.permissions` 含 `database:read` |
| 只读 Key `database list` | 退出码 0 |
| 只读 Key `database create` | 非 0（权限不足） |
| `apikey revoke` 之后再用该 Key | `database list` 失败 |
| `user delete` | `ok=true` |

口令和 Key 明文不出现在 `RESULT_FILE`、终端摘要或文档里。
