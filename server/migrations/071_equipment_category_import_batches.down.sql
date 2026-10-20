-- 95-1 롤백. 운영 데이터는 이 파일로 지우지 않는다. 테스트 DB 전용.

DROP INDEX IF EXISTS ix_ecib_created;
DROP TABLE IF EXISTS equipment_category_import_batches;
