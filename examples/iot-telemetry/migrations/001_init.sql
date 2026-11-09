CREATE TABLE IF NOT EXISTS devices (id VARCHAR, name VARCHAR, model VARCHAR);
CREATE TABLE IF NOT EXISTS readings (device_id VARCHAR, ts VARCHAR, metric VARCHAR, value DOUBLE);
CREATE TABLE IF NOT EXISTS rollup_5m (device_id VARCHAR, bucket VARCHAR, metric VARCHAR, avg DOUBLE, max DOUBLE, count BIGINT, source_run_id VARCHAR);
CREATE TABLE IF NOT EXISTS processed_runs (run_id VARCHAR, processed_at VARCHAR);
