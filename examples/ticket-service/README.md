# 工单服务案例

可信 Node BFF/JS SDK 用于工单 SQL/事件流，项目 KV 保存待办派生缓存，单文件 Go 云函数 `ex_ticket_sla` 和 UTC 每小时定时任务生成 SLA 超时结果。仅供单用户本地演示，前端不持项目 Key；公网身份认证、审计及限流需另行设计。

在 SimpleBase 普通项目创建用户数据库，先于 `packages/js-sdk` 运行 `npm run build`，在可信终端设置 `SIMPLEBASE_URL`、`SIMPLEBASE_PROJECT_ID`、`SIMPLEBASE_DATABASE_ID`、`SIMPLEBASE_API_KEY`（数据库读写、KV 与对象存储上传权限）。然后在本目录运行：

```sh
npm test
npm run build
npm start
npm run publish
npm run publish -- --upload
```

BFF 默认监听 `127.0.0.1:8095`；构建后直接打开 `http://127.0.0.1:8095/`，BFF 同端口提供前端和 API；业务请求需携带精确同源 `Origin`。只有单实例设置 `EXAMPLE_WORKER=1`，才会读取实际 cron runs，查询数据库工单后调用 `Compute` 并按 run ID 回写 SLA 表；否则 cron 仅输出时间窗。`dist/` 上传至项目**私有**对象存储，不会直接成为公开网站。worker 无法追赶 100 条 run、结果无法解析或输入超限时停止，人工检查并按窗口补算。

回滚：先停 worker，再停用 `ex_ticket_sla_scan`、恢复旧版函数；仅删除本次明确列出的工单前端对象。不要自动删除用户库。
