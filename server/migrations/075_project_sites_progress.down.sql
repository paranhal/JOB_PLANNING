-- 95-3 롤백. 운영 데이터는 이 파일로 지우지 않는다. 테스트 DB 전용.
-- customers.org_code · project_sites 설치/검수 열 · status CHECK 는 SQLite에서 제거하지 않는다.

DROP TRIGGER IF EXISTS trg_ps_org_ins;
DROP TRIGGER IF EXISTS trg_ps_org_upd;
DROP INDEX IF EXISTS ix_sib_project;
DROP TABLE IF EXISTS site_import_batches;
DROP INDEX IF EXISTS ix_pse_project;
DROP INDEX IF EXISTS ix_pse_site;
DROP TABLE IF EXISTS project_site_events;
DROP INDEX IF EXISTS ix_ps_project_customer;
DROP INDEX IF EXISTS ix_cust_org_code;
