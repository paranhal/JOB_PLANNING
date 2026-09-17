-- 047 롤백. 배정 알림 표를 없앤다.

DROP INDEX IF EXISTS idx_work_assign_notices_source;
DROP INDEX IF EXISTS idx_work_assign_notices_unseen;
DROP TABLE IF EXISTS work_assign_notices;
DELETE FROM schema_migrations WHERE version = 47;
