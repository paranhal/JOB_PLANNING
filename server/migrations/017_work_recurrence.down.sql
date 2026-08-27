-- 017 롤백. 실행 작업 행은 지우지 않는다. 연결 컬럼·규칙 표만 뗀다.

DROP INDEX IF EXISTS idx_work_tasks_recurrence_parent;
DROP TABLE IF EXISTS work_recurrence;

ALTER TABLE work_tasks DROP COLUMN not_done_reason;
ALTER TABLE work_tasks DROP COLUMN occurrence_status;
ALTER TABLE work_tasks DROP COLUMN occurrence_seq;
ALTER TABLE work_tasks DROP COLUMN recurrence_role;

DELETE FROM id_sequences WHERE seq_key = '__meta:work_recurrence_v1';
