-- 迁移只追加不修改：dev replay 用 _migrations 表按文件名记账。
CREATE TABLE IF NOT EXISTS dev_events___SUFFIX__ (
    id VARCHAR,
    kind VARCHAR,
    created_at VARCHAR
);
