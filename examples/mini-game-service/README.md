# 小游戏服务案例

Go SDK 可信 BFF 管理答题对局、SQL 成绩、KV 5 分钟会话/排行榜；云函数 `ex_game_season` 执行 UTC 小时窗与积分排名计算，定时任务 `ex_game_rank_refresh` 的运行记录由可信 worker 读取并按 run ID 幂等写回用户库。

要求 Go 1.25+，在 SimpleBase 普通项目创建用户数据库，在可信终端设置 `SIMPLEBASE_URL`、`SIMPLEBASE_API_KEY`、`SIMPLEBASE_PROJECT_ID`、`SIMPLEBASE_DATABASE_ID`（数据库读写、KV 以及上传对象权限）。从**仓库根目录**运行：

```sh
go test ./examples/mini-game-service/... ./examples/shared/goexample/...
go run ./examples/shared/goexample/cmd/build ./examples/mini-game-service
go run ./examples/mini-game-service/server
go run ./examples/shared/goexample/cmd/publish mini-game-service ./examples/mini-game-service
go run ./examples/shared/goexample/cmd/publish mini-game-service ./examples/mini-game-service --upload
```

默认后端地址 `127.0.0.1:8094`；从仓库根目录构建后直接打开 `http://127.0.0.1:8094/`，BFF 同源提供前端与 API 并校验 `Origin`。设置 `EXAMPLE_WORKER=1` 仅启动**一个**有权限的 worker 实例，否则 cron 只产生时间窗。对象上传仅是私有构建归档，非公开网站。此演示非生产反作弊系统，不应对外开放匿名写接口。

回滚：停止 worker、停用 `ex_game_rank_refresh`、恢复旧云函数版本，清理确认的 `mini-game-service/<build-id>/` 私有对象；用户库不自动清空。
