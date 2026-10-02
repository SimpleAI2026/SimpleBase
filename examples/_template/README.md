# __name__ 案例（由 ./build.sh init-example 生成）

可信 Node BFF + JS SDK：用户数据库存事项表，项目 KV 记录访问计数；单文件云函数 `ex___prefix___job` 的 `Tick` 返回上一 UTC 小时窗口，每小时定时任务 `ex___prefix___tick` 触发后，由单实例 worker 查询真实数据、调用 `Compute` 并按 run ID 幂等写回小时汇总。浏览器不持 SimpleBase Key；`dist/` 仅上传项目**私有**对象存储，不是公开网站。仅供本地单用户教学。

## 准备

1. 从仓库根目录执行 `cd packages/js-sdk && npm run build`（首次需先安装既有依赖）。
2. 启动 SimpleBase，在**普通项目**创建用户数据库（不要用 admin 系统项目）。
3. 在可信终端设置环境变量：`SIMPLEBASE_URL`、`SIMPLEBASE_API_KEY`、`SIMPLEBASE_PROJECT_ID`、`SIMPLEBASE_DATABASE_ID`（Key 需数据库读写、KV 及对象上传权限）。不要把 Key 写入前端或 `VITE_*`。

## 运行

```sh
cd examples/__name__
npm test
npm run build
npm start
```

打开 `http://127.0.0.1:8100/`；BFF 同源提供页面与 API，并校验 `Origin`。如端口冲突可用 `EXAMPLE_PORT` 覆盖。

## 发布云函数与定时任务

```sh
npm run publish              # 创建/试跑/激活 ex___prefix___job 并配置 ex___prefix___tick
npm run publish -- --upload  # 另将 dist/ 上传至项目私有对象存储 <name>/<build-id>/
```

设置 `EXAMPLE_WORKER=1 npm start`（仅单实例）消费 cron 运行记录：worker 读取 `response_json` 时间窗 → 查询窗口内真实事项 → 调用 `Compute` → 同事务写 `ex___prefix___hourly_stats` 与 run 消费标记。运行记录接口最多返回 100 条且无游标；出现积压、截断或超限时 worker 停止并要求按时间窗人工补算。

## 回滚

停 worker；在控制台停用 `ex___prefix___tick`；必要时重新激活旧版 `ex___prefix___job`；仅删除确认的 `__name__/<build-id>/` 对象。不要删除整个项目或用户库，业务数据人工核对后清理。
