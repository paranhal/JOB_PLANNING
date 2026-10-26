-- 95-58 작업 조치 화면. 시드 INSERT 없음. 기존 행은 기본값.

ALTER TABLE work_tasks ADD COLUMN action_start_at TEXT NOT NULL DEFAULT '';
ALTER TABLE work_tasks ADD COLUMN action_duration_min INTEGER NOT NULL DEFAULT 0;
ALTER TABLE work_tasks ADD COLUMN action_remark TEXT NOT NULL DEFAULT '';
ALTER TABLE work_tasks ADD COLUMN recurrence_possible INTEGER NOT NULL DEFAULT 0;
ALTER TABLE work_tasks ADD COLUMN customer_confirmed_at TEXT NOT NULL DEFAULT '';
