DROP INDEX IF EXISTS ix_wts_site;
DROP TABLE IF EXISTS work_task_sites;
-- SQLite 는 열 삭제를 하지 않는다. work_subtype 값은 비운다.
UPDATE work_tasks SET work_subtype='' WHERE TRIM(COALESCE(work_subtype,'')) != '';
