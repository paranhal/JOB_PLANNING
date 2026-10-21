-- 95-11A 작업 조치 대상 분류·사유. 시드 INSERT 없음.

ALTER TABLE work_tasks ADD COLUMN target_category_id TEXT NOT NULL DEFAULT '';
ALTER TABLE work_tasks ADD COLUMN work_reason TEXT NOT NULL DEFAULT '';
