-- 95-8 롤백. 운영 데이터는 이 파일로 지우지 않는다. 테스트 DB 전용.

DROP INDEX IF EXISTS ix_as_parts_process;
DROP INDEX IF EXISTS ix_as_parts_as;
DROP TABLE IF EXISTS as_parts_used;
