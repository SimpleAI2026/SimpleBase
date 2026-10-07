# 社区

```bash
./examples/init.sh --only community
set -a; . examples/community/.env; set +a
go run ./examples/community/server
```

服务监听 `127.0.0.1:8096`：`GET /api/posts`、`POST /api/posts`（`{"title":"标题","body":"正文"}`）、`POST /api/posts/{id}/likes`、`POST /api/posts/{id}/summary`（无 provider 时 501）、`DELETE /api/posts/{id}`。发帖先调用 `moderation.Check`，文档成功而 SQL 失败时补偿删除文档；重复点赞先查 SQL，事务内再检查。

## 验证清单

控制台 `ex-cmty1` 有 `community` 和 `media` 两库；前者 `posts` 集合里有 demo 帖子，SQL 有 `likes` 和 `post_stats`；后者有 `attachments`；KV 的 Set/ZSet 可读；附件 `posts/demo/welcome.txt`、`moderation.go`、`community_hot` 任务及 completed 记录均可见。未配置 provider 时 LLM 标记“未验证”。

限制：G1、G2、G4、G6、G8、G15（详见 `plan/planv4.0/examples-business-cases-plan.md` §6）。
