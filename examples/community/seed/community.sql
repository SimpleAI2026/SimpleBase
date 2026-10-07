INSERT INTO post_stats SELECT 'demo', 1, 1.0 WHERE NOT EXISTS (SELECT 1 FROM post_stats WHERE post_id = 'demo');
INSERT INTO likes SELECT 'demo', 'demo-user', '2026-10-02T12:00:00Z' WHERE NOT EXISTS (SELECT 1 FROM likes WHERE post_id = 'demo' AND user_id = 'demo-user');
