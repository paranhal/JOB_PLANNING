-- 094 down. SQLite DROP COLUMN 이 되는 환경에서만 컬럼을 제거한다.
-- 기존 업무 행의 다른 컬럼 값은 바꾸지 않는다.

ALTER TABLE work_tasks DROP COLUMN action_start_at;
ALTER TABLE work_tasks DROP COLUMN action_duration_min;
ALTER TABLE work_tasks DROP COLUMN action_remark;
ALTER TABLE work_tasks DROP COLUMN recurrence_possible;
ALTER TABLE work_tasks DROP COLUMN customer_confirmed_at;
