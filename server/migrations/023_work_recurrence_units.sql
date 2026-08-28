-- 023 기간 내 반복 실행 단위 보강 (§33.4)
-- monthly / quarterly / yearly / 지정일자(manual_dates). 실제 적용은 InitDB → applyWorkRecurrence.

ALTER TABLE work_recurrence ADD COLUMN month_day INTEGER NOT NULL DEFAULT 0;
ALTER TABLE work_recurrence ADD COLUMN month_n INTEGER NOT NULL DEFAULT 0;
ALTER TABLE work_recurrence ADD COLUMN last_workday INTEGER NOT NULL DEFAULT 0;
ALTER TABLE work_recurrence ADD COLUMN manual_dates TEXT NOT NULL DEFAULT '';
