# 预约服务案例

通过可信 Node BFF/JS SDK 使用用户数据库、项目 KV 临时暂留、`ex_booking_expiry` 单文件云函数和每 5 分钟定时过期计算。前端只访问本地 BFF，不包含 SimpleBase Key。本例仅为本地单用户教学，**未提供可证明的高并发容量锁和公网认证**，不要用于真实抢号场景。

先构建 `packages/js-sdk`（`npm run build`），在 SimpleBase 普通项目创建用户数据库；在可信终端设置 `SIMPLEBASE_URL`、`SIMPLEBASE_PROJECT_ID`、`SIMPLEBASE_DATABASE_ID`、`SIMPLEBASE_API_KEY`（需要数据库读写和 KV 权限）。在本目录运行：

```sh
npm test
npm run build
npm start
npm run publish
npm run publish -- --upload
```

BFF 默认监听 `127.0.0.1:8093`；先运行 `npm run build`，再打开 `http://127.0.0.1:8093/`；BFF 同端口提供 `dist/` 和业务 API，并检查精确同源 `Origin`。发布 cron 后，单实例执行 `EXAMPLE_WORKER=1 npm start` 消费 runs，并在数据库中按 run ID 防重复处理。`dist/` 上传仅是项目私有对象归档，不是网站托管；公网使用需另配安全静态托管和可信 BFF 鉴权。若 run 超 100 条可见窗口、数据超限或解析失败，worker 停止并应按时间窗从 DB 人工补算。

回滚：停 worker、在控制台停用 `ex_booking_expired`、重新激活旧函数版本；仅清理本案例已确认的 build-id 对象，不自动删业务库。
