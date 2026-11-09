-- 002_audit.sql：触发器回调落库的审计轨迹。
CREATE TABLE IF NOT EXISTS audit_log (id VARCHAR, source VARCHAR, kind VARCHAR, target VARCHAR, action VARCHAR, detail VARCHAR, created_at VARCHAR);