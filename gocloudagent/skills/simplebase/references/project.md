# project
<!-- allow: quota,audit -->
<!-- readonly: quota get,audit list -->

命令：`quota get`、`audit list`。只读，作用于当前项目。

## quota get

```
simplebase quota get
```

无参数。只返回两个布尔值：`llm_allowed`（还能否调用 LLM）与 `database_allowed`（还能否使用数据库），**不含具体用量数字**，不要编造次数或容量。结果有约 10 秒缓存。任一为 `false` 表示该项配额已耗尽，建议用户到控制台处理。

## audit list

```
simplebase audit list --limit 20
```

可选 flag：`--limit`（默认 50，非正整数时回落到默认值）。返回 `{"operations":[...],"limit":N}`，每条含时间、主体、动作与资源标识；审计只记录元数据与 SHA-256，不含 SQL 参数或对话正文，查不到正文属正常。

## 边界

用户、项目、API Key、设置管理不在此 skill 内，助手不做账号或凭据管理；被要求签发或查看密钥时，如实拒绝并指引用户去控制台「设置」。常见错误码：`unauthenticated`（令牌过期，不要重试）。
