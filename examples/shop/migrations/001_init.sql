CREATE TABLE IF NOT EXISTS products (id VARCHAR, sku VARCHAR, name VARCHAR, price_minor BIGINT, stock BIGINT, image_key VARCHAR);
CREATE TABLE IF NOT EXISTS orders (id VARCHAR, user_id VARCHAR, status VARCHAR, total_minor BIGINT, idem_key VARCHAR, created_at VARCHAR);
CREATE TABLE IF NOT EXISTS order_items (order_id VARCHAR, product_id VARCHAR, qty BIGINT, price_minor BIGINT);
CREATE TABLE IF NOT EXISTS daily_stats (day VARCHAR, orders BIGINT, gmv_minor BIGINT, source_run_id VARCHAR);
