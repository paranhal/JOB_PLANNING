-- 95-85 down. 운영 행 값은 보정하지 않는다.
-- SQLite는 ADD COLUMN 을 되돌릴 수 없어 attachments.action_set_id 는 빈 기본값으로 남긴다.

DROP INDEX IF EXISTS idx_attach_action_set;
DROP INDEX IF EXISTS idx_as_action_sets_as;
DROP TABLE IF EXISTS as_action_sets;
