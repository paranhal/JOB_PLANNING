-- 95-11 down. 이력·자재 표만 제거한다. work_tasks 신규 컬럼은
-- SQLite DROP COLUMN 이 환경마다 달라 유지한다(078 org_id 유지와 같음).

DROP INDEX IF EXISTS ix_wtpu_task;
DROP TABLE IF EXISTS work_task_parts_used;
DROP INDEX IF EXISTS ix_wtp_org;
DROP INDEX IF EXISTS ix_wtp_task;
DROP TABLE IF EXISTS work_task_processes;
