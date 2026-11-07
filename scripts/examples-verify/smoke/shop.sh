#!/usr/bin/env bash
# smoke/shop.sh — 电商 BFF 业务冒烟（planv6.4 §4.4）
#
# 断言：GET /api/products 20 条且带 image_url；同一 Idempotency-Key 下单两次只出一单；
#       库存扣减 1；订单可按 id 查回。
#
# 前置（verify.sh 会 source .env.runtime 后调用）：
#   SIMPLEBASE_URL / SIMPLEBASE_API_KEY / SIMPLEBASE_PROJECT_ID / SIMPLEBASE_DATABASE_ID
# 用法：
#   SMOKE_STEP=S8 SMOKE_ENV=A SMOKE_BFF_URL=http://127.0.0.1:28102 bash smoke/shop.sh
#   （$1 可覆盖 BFF URL）

set -uo pipefail
# shellcheck source=../lib.sh
. "$(cd "$(dirname "$0")" && pwd)/../lib.sh"
SV_SUITE="${SV_SUITE:-smoke}"

: "${SIMPLEBASE_URL:?需要 SIMPLEBASE_URL（先 source 运行时 env）}"
: "${SIMPLEBASE_API_KEY:?需要 SIMPLEBASE_API_KEY}"
: "${SIMPLEBASE_PROJECT_ID:?需要 SIMPLEBASE_PROJECT_ID}"
: "${SIMPLEBASE_DATABASE_ID:?需要 SIMPLEBASE_DATABASE_ID（单库站点）}"

BFF="${1:-${SMOKE_BFF_URL:-}}"
if [ -z "$BFF" ]; then
  echo "smoke/shop.sh: 需要 BFF URL（参数或 SMOKE_BFF_URL）" >&2
  exit 1
fi
BFF="${BFF%/}"

smoke_start
smoke_init_tmp
SMOKE_STARTED="$(now_ms)"
SQL_DB="$SIMPLEBASE_DATABASE_ID"

# ── 1. BFF 健康 ─────────────────────────────────────────
code="$(sb_request GET "$BFF/health")"
[ "$code" = "200" ] && [ "$(sb_body | jq -r '.ok // false' 2>/dev/null)" = "true" ]
smoke_check "bff-health" $? "code=$code"

# ── 2. 商品 20 条且带预签名 image_url ────────────────────
code="$(sb_request GET "$BFF/api/products")"
count="$(sb_body | jq 'length' 2>/dev/null || echo 0)"
missing="$(sb_body | jq '[.[] | select((.image_url // "") == "")] | length' 2>/dev/null || echo -1)"
[ "$code" = "200" ] && [ "$count" = "20" ] && [ "$missing" = "0" ]
smoke_check "products-20-with-image_url" $? "code=$code count=$count missing_url=$missing"

# ── 3. 同一 Idempotency-Key 下单两次只出一单 ────────────
stock_before="$(sql_scalar "$SQL_DB" "SELECT stock FROM products WHERE sku = 'sku-01'")"
idem="$(python3 -c 'import uuid; print(uuid.uuid4())')"
payload='{"productId":"sku-01","qty":1}'

code1="$(sb_request POST "$BFF/api/orders" "$payload" "$SIMPLEBASE_API_KEY" -H "Idempotency-Key: $idem")"
order1="$(sb_body | jq -r '.id // empty' 2>/dev/null)"
code2="$(sb_request POST "$BFF/api/orders" "$payload" "$SIMPLEBASE_API_KEY" -H "Idempotency-Key: $idem")"
order2="$(sb_body | jq -r '.id // empty' 2>/dev/null)"
orders="$(sql_scalar "$SQL_DB" "SELECT COUNT(*) FROM orders WHERE idem_key = '$idem'")"
[ "$code1" = "201" ] && [ "$code2" = "201" ] && [ -n "$order1" ] && [ "$order1" = "$order2" ] && [ "$orders" = "1" ]
smoke_check "idempotent-order" $? "first=$code1 second=$code2 same_id=$([ "$order1" = "$order2" ] && echo yes || echo no) orders=$orders"

# ── 4. 库存扣减 1 ───────────────────────────────────────
stock_after="$(sql_scalar "$SQL_DB" "SELECT stock FROM products WHERE sku = 'sku-01'")"
[ -n "$stock_before" ] && [ -n "$stock_after" ] && [ "$((stock_before - stock_after))" = "1" ]
smoke_check "stock-decrement-1" $? "before=$stock_before after=$stock_after"

# ── 5. 订单可按 id 查回 ─────────────────────────────────
code="$(sb_request GET "$BFF/api/orders/$order1")"
[ "$code" = "200" ] && [ "$(sb_body | jq -r '.id // empty' 2>/dev/null)" = "$order1" ]
smoke_check "get-order-by-id" $? "code=$code"

# ── 6. 统计接口（daily_stats） ──────────────────────────
code="$(sb_request GET "$BFF/api/stats")"
[ "$code" = "200" ] && sb_body | jq -e 'type == "array"' >/dev/null 2>&1
smoke_check "stats" $? "code=$code"

SMOKE_MS=$(( $(now_ms) - SMOKE_STARTED ))
extra="product sku-01 stock ${stock_before}→${stock_after}，order ${order1:0:8}"
smoke_finish "$extra"
