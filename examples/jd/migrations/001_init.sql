-- 京东示例：商品、交易、营销、评价的最小可用表结构。
-- DuckLake 不支持主键/唯一索引，唯一性由业务写入逻辑（幂等键、先查后写）保证。
CREATE TABLE IF NOT EXISTS categories (id VARCHAR, name VARCHAR, icon VARCHAR, sort_order BIGINT);

CREATE TABLE IF NOT EXISTS products (id VARCHAR, sku VARCHAR, name VARCHAR, subtitle VARCHAR, category_id VARCHAR, price_minor BIGINT, orig_price_minor BIGINT, stock BIGINT, sales BIGINT, rating DOUBLE, review_count BIGINT, image_key VARCHAR, tags VARCHAR, specs VARCHAR, is_flash BOOLEAN, flash_price_minor BIGINT, flash_end_at VARCHAR, created_at VARCHAR);

CREATE TABLE IF NOT EXISTS users (id VARCHAR, phone VARCHAR, nickname VARCHAR, created_at VARCHAR);

CREATE TABLE IF NOT EXISTS addresses (id VARCHAR, user_id VARCHAR, receiver VARCHAR, phone VARCHAR, province VARCHAR, city VARCHAR, district VARCHAR, detail VARCHAR, is_default BOOLEAN, created_at VARCHAR);

CREATE TABLE IF NOT EXISTS cart_items (id VARCHAR, user_id VARCHAR, product_id VARCHAR, qty BIGINT, checked BOOLEAN, created_at VARCHAR);

CREATE TABLE IF NOT EXISTS favorites (id VARCHAR, user_id VARCHAR, product_id VARCHAR, created_at VARCHAR);

CREATE TABLE IF NOT EXISTS coupons (id VARCHAR, code VARCHAR, title VARCHAR, kind VARCHAR, value BIGINT, min_minor BIGINT, total BIGINT, remain BIGINT, start_at VARCHAR, end_at VARCHAR);

CREATE TABLE IF NOT EXISTS user_coupons (id VARCHAR, user_id VARCHAR, coupon_id VARCHAR, status VARCHAR, order_id VARCHAR, claimed_at VARCHAR, used_at VARCHAR);

CREATE TABLE IF NOT EXISTS orders (id VARCHAR, user_id VARCHAR, status VARCHAR, subtotal_minor BIGINT, discount_minor BIGINT, freight_minor BIGINT, pay_minor BIGINT, address_id VARCHAR, address_text VARCHAR, coupon_id VARCHAR, idem_key VARCHAR, remark VARCHAR, created_at VARCHAR, paid_at VARCHAR, canceled_at VARCHAR);

CREATE TABLE IF NOT EXISTS order_items (order_id VARCHAR, product_id VARCHAR, sku VARCHAR, name VARCHAR, price_minor BIGINT, qty BIGINT, image_key VARCHAR);

CREATE TABLE IF NOT EXISTS reviews (id VARCHAR, product_id VARCHAR, user_id VARCHAR, nickname VARCHAR, rating BIGINT, content VARCHAR, created_at VARCHAR);

CREATE TABLE IF NOT EXISTS daily_stats (day VARCHAR, orders BIGINT, gmv_minor BIGINT, buyers BIGINT, source_run_id VARCHAR);