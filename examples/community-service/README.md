# 内容社区案例

可信 Go SDK BFF 使用用户数据库内的 `ex_community_posts` 文档集合、SQL 点赞和项目 KV 热榜；云函数 `ex_community_trending` 对脱敏计数计算热度，UTC 每小时任务 `ex_community_hot` 的时间窗由可信 worker 消费并写回 SQL。

要求 Go 1.25+。在 SimpleBase 普通项目创建用户数据库，设置 `SIMPLEBASE_URL`、`SIMPLEBASE_API_KEY`、`SIMPLEBASE_PROJECT_ID`、`SIMPLEBASE_DATABASE_ID`，仅在可信环境注入数据库读写、KV、对象上传所需 Key。从仓库根目录运行：

```sh
go test ./examples/community-service/... ./examples/shared/goexample/...
go run ./examples/shared/goexample/cmd/build ./examples/community-service
go run ./examples/community-service/server
go run ./examples/shared/goexample/cmd/publish community-service ./examples/community-service
go run ./examples/shared/goexample/cmd/publish community-service ./examples/community-service --upload
```

后端只监听 `127.0.0.1:8096`。从仓库根目录构建后直接打开 `http://127.0.0.1:8096/`；BFF 同源提供 `dist/` 和业务 API，并校验 `Origin`。仅单实例设置 `EXAMPLE_WORKER=1` 才消费 cron 运行记录；如果出现运行记录积压或大输入，先停止并人工对账。文档与 SQL 点赞不具有跨模型事务，演示要避免用于需要严格跨模型原子性的生产业务；公网身份认证与滥用防护不在例子范围。私有对象上传不等于公开网站部署。

回滚：停止 worker、停用 `ex_community_hot`，恢复已记录的旧云函数版本，仅删除确认的本案例 build-id 对象，业务数据须人工核对后处理。
