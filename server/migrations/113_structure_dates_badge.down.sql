DROP INDEX IF EXISTS idx_structure_date_changes_project;
DROP TABLE IF EXISTS structure_date_changes;
-- SQLite 는 기존 표의 추가 컬럼(phase_packages.plan_start/plan_end/actual_start/badge)을 DROP COLUMN 하지 않는다.
-- 운영 되돌리기는 DB 백업 복원이다.
