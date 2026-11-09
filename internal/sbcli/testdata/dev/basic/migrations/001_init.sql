-- 迁移只追加不修改。
CREATE TABLE IF NOT EXISTS notes (id VARCHAR, title VARCHAR, created_at VARCHAR);
CREATE TABLE IF NOT EXISTS tags (id VARCHAR, note_id VARCHAR, name VARCHAR);
