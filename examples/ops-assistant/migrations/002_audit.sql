-- 002_audit.sql：触发器回落的审计轨迹。
CREATE TABLE IF NOT EXISTS audit_log (id VARCHAR, source VARCHAR, kind VARCHAR, target VARCHAR, action VARCHAR, detail VARCHAR, created_at VARCHAR);

-- 003 runbook 附件元数据（对象存储前缀 ops/attachments/）。
CREATE TABLE IF NOT EXISTS runbook_assets (key VARCHAR, ticket_id VARCHAR, size BIGINT, mime VARCHAR, created_at VARCHAR);