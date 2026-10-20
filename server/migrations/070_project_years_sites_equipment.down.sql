-- 95 롤백. 운영 데이터는 이 파일로 지우지 않는다. 테스트 DB 전용.

DROP TRIGGER IF EXISTS trg_cy_overlap_upd;
DROP TRIGGER IF EXISTS trg_cy_overlap_ins;
DROP TABLE IF EXISTS equipment_import_batches;
DROP TABLE IF EXISTS project_equipment_values;
DROP TABLE IF EXISTS project_equipment;
DROP TABLE IF EXISTS equipment_category_fields;
DROP TABLE IF EXISTS equipment_categories;
DROP TABLE IF EXISTS project_sites;
DROP TABLE IF EXISTS contract_years;
