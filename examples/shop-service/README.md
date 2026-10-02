# 商城服务案例

本地可信 Node BFF + JS SDK + DuckLake 商品/订单 + KV 购物车 + Go 云函数/cron。浏览器不持 SimpleBase Key；`dist/` 只上传项目**私有**对象存储，不是可公开访问的网站。仅用于单用户本地教学，不提供公网认证/限流/反欺诈能力。

## 准备与运行

要求 Node 18+；从仓库根目录执行 `cd packages/js-sdk && npm run build`（首次按仓库安装说明先安装既有依赖）。启动 SimpleBase，在**普通项目**创建用户数据库，设置 `SIMPLEBASE_URL`、`SIMPLEBASE_API_KEY`、`SIMPLEBASE_PROJECT_ID`、`SIMPLEBASE_DATABASE_ID`；Key 需项目数据库读写及对象存储写权限，KV 当前路由也要求数据库写权限。不得提交真实 Key 或将其放入 `VITE_*` 环境变量。

```sh
cd examples/shop-service
npm test
npm run build
npm start
```

服务默认 `http://127.0.0.1:8092`，仅监听环回地址。先执行 `npm run build`，然后直接打开 `http://127.0.0.1:8092/`；BFF 在同源端口提供 `dist/` 与业务 API，拒绝来源端口不同的业务请求。部署云函数及任务：`node deploy/publish.mjs`；仅在准备好私有项目对象存储后运行 `node deploy/publish.mjs --upload` 上传静态文件。设置 `EXAMPLE_WORKER=1` 再启动 BFF，单实例消费 cron run。管理端发布写权限 Key 必须只留在可信机器。

订单存储使用参数化 SQL 和事务，KV 保存 30 分钟购物车。云函数 `Tick` 只返回前一 UTC 日窗口，worker 查真正的订单并调用 `Compute` 汇总后按 run ID 写库。运行记录最多返回 100 条且没有游标；有积压、截断或超限时 worker 停止并要求人工按窗口补算。此例未验证多写实例库存竞争，**不得直接用作生产商城**。

## 回滚

先关闭 `EXAMPLE_WORKER`，在控制台停用 `ex_shop_daily`，必要时重新激活旧版 `ex_shop_metrics`，仅删除已核对的 `shop-service/<build-id>/` 对象清单；不要删除整个项目或用户库。业务数据的清理应在检查后人工进行。
