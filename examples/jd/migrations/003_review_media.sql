-- 003_review_media.sql：评价支持晒图（图片存在对象存储 jd/reviews/ 前缀）。
ALTER TABLE reviews ADD COLUMN IF NOT EXISTS image_key VARCHAR;