DROP INDEX IF EXISTS idx_pa_user;
DROP INDEX IF EXISTS idx_pa_project;
DROP TABLE IF EXISTS project_assignments;
DELETE FROM codes WHERE code_id IN ('PKC01','PKC02','PKC03','PKC04');
ALTER TABLE work_projects DROP COLUMN project_kind;
