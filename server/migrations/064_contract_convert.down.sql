-- SQLite 버전에 따라 DROP COLUMN 이 된다. 안 되면 시드만 지우고 컬럼은 둔다.

DROP INDEX IF EXISTS idx_wp_contract_category;
DROP INDEX IF EXISTS idx_wp_contract_date;
DELETE FROM codes WHERE code_group='contract_category';

ALTER TABLE work_projects DROP COLUMN amount_vat_included;
ALTER TABLE work_projects DROP COLUMN contract_category;
ALTER TABLE work_projects DROP COLUMN contract_date;
