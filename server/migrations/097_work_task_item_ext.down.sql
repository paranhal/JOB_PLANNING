-- 95-82 down. 운영 행 값은 보정하지 않는다.
-- SQLite는 ADD COLUMN 을 되돌릴 수 없어 work_task_items 확장 컬럼은 빈 기본값으로 남긴다.

DROP INDEX IF EXISTS idx_wtia_asset;
DROP TABLE IF EXISTS work_task_item_assets;
DROP TABLE IF EXISTS work_task_item_assignees;
DROP INDEX IF EXISTS idx_wti_as;
DELETE FROM codes WHERE code_group='wb_item_kind' AND code_id IN ('WIK01','WIK02','WIK03','WIK04');
