# 电商

```bash
./examples/init.sh --only shop
set -a; . examples/shop/.env; set +a
node examples/shop/server/main.mjs
```

浏览 `http://127.0.0.1:8092`，调用 `POST /api/orders` 时传 `Idempotency-Key: <UUID>`、`{"productId":"sku-01","qty":1}`。

## 能力覆盖

| 能力 | 落在哪 |
| --- | --- |
| 数据库（SQL） | `products`、`orders`、`order_items`、`daily_stats`、`audit_log` |
| 集合文档 | `shop_events`（订单事件流，驱动 record_change 触发器） |
| KV | `cart:demo`（hash + TTL）、`cache:hot`（JSON 字符串） |
| 对象存储 | `products/*.svg`（20 张商品图，页面走 `s3/presign` 取 URL） |
| 云函数 | `pricing.Quote` 算价、`pricing.Tick` 生成统计窗口；`audit.OnRecord/OnObject/OnHttp` 转发事件 |
| 定时任务 | `shop_daily`（每天 01:00，cron），有一条 completed 运行记录 |
| 事件触发器 | `record_change:shop_events`、`object_event:products/`、`http` 三类 |
| 事务 | 下单扣库存、写订单、写明细在一个 `sql.batch({ transactional: true })` 里 |
| 幂等 | `Idempotency-Key` + KV `SET NX EX` 抢锁 + `orders.idem_key` 复用 |

页面上的「事件触发器」区块可以写订单事件、上传对象、invoke HTTP 触发器；
「触发器审计」读 `audit_log`，能看到云函数回调落库的每一次调用。

## 验证清单

控制台切到 `ex-shop1`：

- `shop` 数据库含 20 条 `products`、`orders`、`order_items`、`daily_stats` 与 `_migrations`（含 `002_audit.sql`）。
- 集合 `shop_events` 有种子文档 `evt-demo-0001`。
- KV 有 `cart:demo`（hash，TTL 1800s）、`cache:hot`。
- 对象 `products/` 下 20 张商品图。
- 云函数 `pricing`、`audit` 各有一个生效版本，试跑均通过。
- 定时任务 `shop_daily` 有一条 completed 记录。
- `GET /v1/projects/ex-shop1/function-triggers`（需要 `project:admin`）能看到三条触发器；运行时的 `examples/shop/.simplebase/triggers.json` 是脚本写下的只读快照。

BFF 在下单时同步调用云函数 `pricing.Quote` 算价，同一幂等键只生成一单。

## 发布到子域名站点

`web/` 是纯静态目录，但页面调的 `/api/*` 在 `server/main.mjs`（BFF）里，**站点面只读、没有后端接口**，BFF 必须另起：

1. 启动 BFF：`node examples/shop/server/main.mjs`（默认 `EXAMPLE_PORT=8092`）。
2. 把 `web/config.js` 改成 `window.__SB_CONFIG__ = { apiBase: 'http://127.0.0.1:8092' }`（只放 BFF 地址，不放 Key；默认空串 = 同源）。
3. 复制到临时目录后发布：`python3 skills/simplebase/scripts/sbdev.py site deploy -C <临时目录> --dist web --prefix web/`，访问 `http://ex-shop1.localhost:8080/web/`。不带 `--prefix` 会公开整个对象根（含 `products/` 商品图）。

BFF 的 CORS 为 `Access-Control-Allow-Origin: *`，方法与请求头全部允许，且没有登录与 Origin 校验：**只适合本地与演示，不要绑定到公网地址。**

限制：平台能力缺口见 `openspec/changes/close-legacy-backlog`。