-- 95-37 AS 담당 그룹 + 구성원 기간 조회 인덱스.

ALTER TABLE as_receipts ADD COLUMN work_group_id TEXT;

CREATE INDEX IF NOT EXISTS ix_as_receipts_work_group ON as_receipts(work_group_id);
CREATE INDEX IF NOT EXISTS ix_wgm_user_group_dates ON work_group_members(user_id, group_id, joined_on, left_on);
