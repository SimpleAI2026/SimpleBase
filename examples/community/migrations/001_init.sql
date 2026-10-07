CREATE TABLE IF NOT EXISTS likes (post_id VARCHAR, user_id VARCHAR, created_at VARCHAR);
CREATE TABLE IF NOT EXISTS post_stats (post_id VARCHAR, likes BIGINT, score DOUBLE);
