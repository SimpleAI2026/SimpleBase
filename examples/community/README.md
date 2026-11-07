# 社区

```bash
./examples/init.sh --only community
set -a; . examples/community/.env; set +a
go run ./examples/community/server
```

浏览 `http://127.0.0.1:8096`。

## 能力覆盖

| 能力 | 落在哪 |
| --- | --- |
| 数据库（SQL） | `post_stats`、`likes`、`attachments`、`audit_log`（文档与 SQL 同库双模型） |
| 集合文档 | `posts`（种子文档 `demo`）、`platform_events`（平台页写事件驱动触发器） |
| KV | `liked:demo-user`（set）、`rank:hot`（zset）、`post:demo`（string + TTL） |
| 对象存储 | `posts/demo/welcome.txt` 附件，`posts/` 前缀下的对象事件会触发云函数 |
| 云函数 | `moderation.Check/Score`（发帖校验、热榜打分）、`audit.OnRecord/OnObject/OnHttp` |
| 定时任务 | `community_hot`（每 600s），有一条 completed 运行记录 |
| 事件触发器 | `record_change:posts`、`record_change:platform_events`、`object_event:posts/`、`http` |
| LLM | `POST /api/posts/{id}/summary` 走 OpenAI 兼容出口，无 provider 时返回 501 |

BFF 额外提供：

```bash
curl localhost:8096/api/platform            # KV / 对象 / 定时任务 / 触发器 / 审计总览
curl -X POST localhost:8096/api/platform/emit-event -d '{"detail":"hi"}'
curl -X POST localhost:8096/api/platform/upload -d '{"key":"posts/demo/note.txt"}'
curl -X POST localhost:8096/api/platform/invoke-trigger
curl -X POST localhost:8096/api/platform/run-cron
```

## 验证清单

控制台 `ex-cmty1` 只有一个 `community` 库：文档集合 `posts`（种子 `demo`）与 `platform_events`，
SQL 表 `post_stats`、`likes`、`attachments`、`audit_log` 与 `_migrations`；
KV 三个键；对象 `posts/demo/welcome.txt`；云函数 `moderation`、`audit`；
定时任务 `community_hot` 有 completed 记录；四条触发器（三条管理面可查，一条 http）。

附件上传与发帖会写 `audit_log`，页面与 `/api/platform` 都能看到调用次数。

限制：LLM 相关断言在未配置 provider 时按 SKIP 处理；其余见 G1、G2、G6、G10、G13。