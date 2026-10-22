-- SQLite 는 컬럼 DROP 이 불안정하다. 081 컬럼은 NULL 허용이라 남겨 둔다.
DROP INDEX IF EXISTS ix_work_tasks_group;
SELECT 1;
