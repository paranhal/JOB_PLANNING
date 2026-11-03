-- 95-83 down. 운영 행 값은 보정하지 않는다.
-- SQLite는 ADD COLUMN 을 되돌릴 수 없어 work_task_items 확장 컬럼은 빈 기본값으로 남긴다.

DROP INDEX IF EXISTS idx_wb_item_tpl_link_child;
DROP TABLE IF EXISTS wb_item_template_links;
DROP INDEX IF EXISTS idx_wb_item_tpl_org;
DROP TABLE IF EXISTS wb_item_templates;
DELETE FROM codes WHERE code_group='wb_item_unit' AND code_id IN ('WIU01','WIU02','WIU03','WIU04');
