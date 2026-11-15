-- 95-201 되돌리기
DROP INDEX IF EXISTS ix_as_affected_asset_as;
DROP INDEX IF EXISTS ix_as_affected_asset_asset;
DROP INDEX IF EXISTS ux_as_affected_asset;
DROP TABLE IF EXISTS as_receipt_affected_assets;
-- KDevice 원인 분류 제거(이미 AS 에 저장된 cause_cat 코드는 그대로 남는다)
DELETE FROM as_cause_categories WHERE code LIKE 'klas.kdevice%';
DELETE FROM as_cause_category_orgs
 WHERE parent_code IN (SELECT code FROM as_cause_category_orgs WHERE label = 'KDevice(클라이언트 프로그램)')
    OR label = 'KDevice(클라이언트 프로그램)';
