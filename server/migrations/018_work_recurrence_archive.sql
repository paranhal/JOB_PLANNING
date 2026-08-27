-- 018 반복 실행 상위 보관 (§13.15.8)
-- 완료된 실행 작업을 지우지 않고 목록에서만 내린다.

ALTER TABLE work_recurrence ADD COLUMN archived INTEGER NOT NULL DEFAULT 0;
