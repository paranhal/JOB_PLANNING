-- 95-5 롤백. 운영 데이터는 이 파일로 지우지 않는다. 테스트 DB 전용.

DROP INDEX IF EXISTS ix_site_insp_prod;
DROP TABLE IF EXISTS site_inspection_products;
DROP INDEX IF EXISTS ix_insp_prod_org;
DROP TABLE IF EXISTS inspection_products;
