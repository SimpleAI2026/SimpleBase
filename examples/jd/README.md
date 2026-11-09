# 京东商城

仿京东的本地全栈示例：商品/搜索/秒杀、购物车、优惠券、地址、结算下单、订单流转、收藏、评价、经营看板。
数据全部落在本地 SimpleBase（DuckLake + KV + 对象存储），前端不携带 Key。

```bash
./examples/init.sh --only jd
set -a; . examples/jd/.env; set +a
node examples/jd/server/main.mjs
```

浏览 `http://127.0.0.1:8093`。

## 页面与接口

| 页面 | 路由（hash） | BFF |
| --- | --- | --- |
| 首页（分类、热门搜索、秒杀、推荐） | `#/` | `GET /api/home` |
| 商品列表（搜索、排序、分页） | `#/products?category=&keyword=&sort=&page=` | `GET /api/products` |
| 商品详情（规格、评价、同类推荐） | `#/product/<sku>` | `GET /api/products/<sku>` |
| 购物车（勾选、加减、删除、全选） | `#/cart` | `GET/POST/PATCH/DELETE /api/cart` |
| 结算抽屉（地址、优惠券、算价） | 购物车「去结算」 | `POST /api/price` |
| 下单（事务 + 幂等键） | — | `POST /api/orders` |
| 订单列表/详情（付款、收货、取消） | `#/orders`、`#/order/<id>` | `GET /api/orders`、`POST /api/orders/<id>/{pay,complete,cancel}` |
| 优惠券（我的券、领券中心） | `#/coupons` | `GET /api/coupons`、`POST /api/coupons/<code>` |
| 收货地址（新增、设默认） | `#/addresses` | `GET/POST /api/addresses`、`POST /api/addresses/<id>/default` |
| 我的关注 | `#/favorites` | `GET/POST /api/favorites` |
| 经营看板 | `#/stats` | `GET /api/stats` |

写评价在商品详情页底部：`POST /api/reviews`（提交后重算商品平均分）。

## 能力覆盖

| 能力 | 落在哪 |
| --- | --- |
| 数据库（SQL） | `products`、`orders`、`order_items`、`reviews`、`coupons`、`addresses`、`daily_stats`、`audit_log` 等 11 张表 |
| 集合文档 | `jd_events`（订单/支付/收货/取消事件流，驱动 record_change 触发器） |
| KV | `jd:hot:search`、`jd:flash:end`、`jd:seckill:lock`、下单幂等锁 `jd:order:idem:*` |
| 对象存储 | `jd/*.svg` 商品图、`jd/reviews/*` 评价晒图（`object_event:jd/reviews/` 触发审计） |
| 云函数 | `pricing.Quote/Tick`、`recommend.Rank`、`audit.OnRecord/OnObject/OnHttp` |
| 定时任务 | `jd_daily`（每天 02:00 cron），有 completed 运行记录 |
| 事件触发器 | `record_change:jd_events`、`object_event:jd/reviews/`、`http` |
| 事务与幂等 | 下单在 `sql.batch({transactional:true})` 里扣库存、写单、锁券、清车；UUID 幂等键 + KV 抢锁 |

页面顶部「平台能力」(`#/platform`) 展示 KV 键值与 TTL、对象列表、云函数版本与导出、定时任务最近状态、
三条触发器及各自调用次数，以及 `audit_log` 的最近 30 条轨迹；三个按钮分别写文档事件、上传对象、invoke HTTP 触发器。

## 关键实现

- **算价在云函数里**：`pricing.Quote`（`functions/pricing.go`）处理满减券、折扣券、包邮门槛（满 99 免运费）与 PLUS 免运费，BFF 只负责取数与落库。
- **首页排序在云函数里**：`recommend.Rank`（`functions/recommend.go`）按销量、评分、价格与秒杀加权打分，同分用 ID 保证稳定顺序，不 import `sort`。
- **下单是一个事务**：扣库存（`stock >= qty` 条件更新）、写订单、写订单行、累加销量、锁定优惠券、清购物车在一个 `sql.batch({ transactional: true })` 里；任一步 `rows_affected` 不符则整体回滚。
- **幂等**：`Idempotency-Key` 必须是 UUID，先查 `orders.idem_key`，再用 KV `SET NX EX 600` 抢锁；重复提交返回同一订单。
- **秒杀价**：`is_flash` 且未过 `flash_end_at` 时展示 `flash_price_minor`，首页倒计时按 `flash_end_at` 走。
- **商品图**：`seed/images/*.svg` 上传到对象前缀 `jd/`，页面通过 `s3/presign` 取 URL（本地模式返回 data URL）。

## 验证清单

控制台切到 `ex-jd001`：

- 库 `jd`：`categories`(8)、`products`(38)、`coupons`(4)、`user_coupons`(2)、`cart_items`(3)、`favorites`(3)、`orders`(2)、`order_items`、`reviews`(5)、`daily_stats`(1)，以及 `_migrations`。
- KV：`jd:hot:search`、`jd:flash:end`、`jd:seckill:lock`。
- 对象：`jd/` 下 38 张商品图。
- 云函数：`pricing`（`Quote` / `Tick` 试跑）、`recommend`（`Rank` 试跑）。
- 定时任务：`jd_daily`（每天 02:00）有一条 completed 运行记录。

命令行自检：

```bash
set -a; . examples/jd/.env; set +a
./simplebase function invoke --name pricing --export Quote \
  --input '{"items":[{"price_minor":399900,"qty":1}],"coupon_kind":"amount","coupon_value":2000,"min_minor":100000}'
./simplebase function invoke --name recommend --export Rank \
  --input '{"seed":"demo","items":[{"id":"a","name":"A","sales":10,"rating":4.5,"price_minor":9900}]}'
./simplebase sql query --database "$SIMPLEBASE_DATABASE_ID" \
  --statement "SELECT status, COUNT(*) FROM orders GROUP BY status"
```

## 限制

- 单用户演示（`user-demo-0001`），没有登录与鉴权；Key 只在服务端 `.env`。
- 不接真实支付，付款是直接把 `pending` 改成 `paid`。
- 库存扣减靠条件更新，不加行锁；高并发下超卖由 `stock >= qty` 与事务失败重试兜住。
- 平台限制编号 G1、G2、G6、G10、G13（详见 `plan/planv4.0/examples-business-cases-plan.md` §6）。