-- 95-13 일일 업무 그룹. 시드 INSERT 없음. FK 없음.

ALTER TABLE work_tasks ADD COLUMN work_group_id TEXT NULL;
CREATE INDEX IF NOT EXISTS ix_work_tasks_group ON work_tasks(work_group_id);
