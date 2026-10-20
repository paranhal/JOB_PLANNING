-- 95-2 롤백. 운영 데이터는 이 파일로 지우지 않는다. 테스트 DB 전용.
-- direction 컬럼은 SQLite에서 제거하지 않는다.

DROP INDEX IF EXISTS ix_cib_created;
DROP TABLE IF EXISTS contract_import_batches;
DROP INDEX IF EXISTS ix_cec_contract;
DROP TABLE IF EXISTS contract_end_customers;
