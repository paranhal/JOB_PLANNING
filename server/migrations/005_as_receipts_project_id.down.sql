-- 005 롤백. as_receipts.project_id 만 제거한다.
-- maintenance_visits.project_id 는 부록 B(S-4) 컬럼이므로 여기서 지우지 않는다.
-- SQLite 3.35+ 의 DROP COLUMN 을 쓴다.

DROP INDEX IF EXISTS idx_as_receipts_project;
ALTER TABLE as_receipts DROP COLUMN project_id;
