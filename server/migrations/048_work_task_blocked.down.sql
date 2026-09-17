-- 048 롤백. SQLite는 DROP COLUMN을 버전마다 지원하므로 표는 두고 값만 비운다.

UPDATE work_tasks SET blocked_reason='', blocked_at='';
DELETE FROM schema_migrations WHERE version = 48;
