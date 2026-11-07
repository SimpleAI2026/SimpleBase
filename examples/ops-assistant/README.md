# 运维助理

```bash
./examples/init.sh --only ops-assistant
set -a; . examples/ops-assistant/.env; set +a
go run ./examples/ops-assistant/server
bash examples/ops-assistant/scripts/verify-perms.sh
```

## 能力覆盖

| 能力 | 落在哪 |
| --- | --- |
| 数据库（SQL） | `tickets`、`ticket_events`、`sla_marks`、`audit_log`、`runbook_assets` |
| 集合文档 | `ticket_meta`（种子 `ticket-demo`）、`ops_events`（驱动 record_change） |
| KV | `idem:demo`（string + TTL）、`queue:pending`（list）、`health:demo`（hash）、`priority:demo`（zset） |
| 对象存储 | `ops/attachments/runbook.txt`，`ops/attachments/` 下的对象事件触发云函数 |
| 云函数 | `sla.Evaluate`（逾期判定）、`sla.Summary`（概览文案）、`audit.OnRecord/OnObject/OnHttp` |
| 定时任务 | `ops_sla_scan`（每小时 cron），有一条 completed 记录 |
| 事件触发器 | `record_change:ops_events`、`object_event:ops/attachments/`、`http` |
| 只读 Key | `.env` 里的 `SIMPLEBASE_READONLY_KEY`，`verify-perms.sh` 逐条验证写操作被拒 |

BFF 提供 `/api/tickets`、`/api/health`、`/api/triggers`（触发器与审计总览）。

## 验证清单

控制台 `ex-ops01` 只有一个 `ops` 库（工单表 `tickets`、事件表、SLA 标记、`audit_log`、`runbook_assets` 与 `_migrations`）、
`ticket_meta` 与 `ops_events` 集合、KV 四个键、`ops/attachments/runbook.txt`、
`sla`/`audit` 两个云函数、`ops_sla_scan` completed 记录、三条触发器。

`.env` 里两把 Key 分别用于服务和只读请求：

```bash
set -a; . examples/ops-assistant/.env; set +a
curl -s "$SIMPLEBASE_URL/v1/projects/$SIMPLEBASE_PROJECT_ID/databases/$SIMPLEBASE_DATABASE_ID/query" \
  -H "authorization: Bearer $SIMPLEBASE_READONLY_KEY" -H 'content-type: application/json' \
  -d '{"sql":"SELECT COUNT(*) FROM tickets"}'
```

`verify-perms.sh` 预期只读 SQL/KV（KV 读命令只要求 `database:read`，G3 已随 M1 修复、函数调用与文件列表成功、写操作 403。跨项目访问需要同实例上存在另一个项目（默认 `ex-shop1`），项目不存在时该用例 SKIP。

LLM provider 未配置时，助手与助手定时任务记「未验证」，不计失败。
限制：G0b、G1、G2、G4、G6、G11、G12（`plan/planv4.0/examples-business-cases-plan.md` §6）。