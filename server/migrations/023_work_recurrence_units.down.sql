-- 023 롤백. 실행 작업 행은 지우지 않는다.

ALTER TABLE work_recurrence DROP COLUMN month_day;
ALTER TABLE work_recurrence DROP COLUMN month_n;
ALTER TABLE work_recurrence DROP COLUMN last_workday;
ALTER TABLE work_recurrence DROP COLUMN manual_dates;
