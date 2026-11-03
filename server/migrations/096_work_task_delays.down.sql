-- 95-80 down. 이력 표만 제거. work_tasks 행 값은 보정하지 않는다.
-- SQLite는 ADD COLUMN 을 되돌릴 수 없어 orig_* / carry_count 컬럼은 빈 기본값으로 남긴다.

DROP INDEX IF EXISTS idx_wtd_org;
DROP INDEX IF EXISTS idx_wtd_task;
DROP TABLE IF EXISTS work_task_delays;
