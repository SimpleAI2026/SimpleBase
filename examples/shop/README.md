# 电商

```bash
./examples/init.sh --only shop
set -a; . examples/shop/.env; set +a
node examples/shop/server/main.mjs
```

浏览 `http://127.0.0.1:8092`，调用 `POST /api/orders` 时传 `Idempotency-Key: <UUID>`、`{"productId":"sku-01","qty":1}`。

## 验证清单

控制台切到 `ex-shop1`：`shop` 数据库含 20 条 `products`、`orders`、`order_items`、`daily_stats` 与 `_migrations`；KV 有 `cart:demo`、`cache:hot`；文件有 `products/*.svg`（20 张）；`pricing.go` 的 `Quote` / `Tick` 可试跑；`shop_daily` 有 completed 记录。BFF 用 `pricing.Quote` 实时算价，同一幂等键只生成一单。

限制：G1、G2、G6、G10、G13（详见 `plan/planv4.0/examples-business-cases-plan.md` §6）。
