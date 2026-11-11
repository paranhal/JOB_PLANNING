DROP INDEX IF EXISTS idx_package_plans_pkg;
DROP TABLE IF EXISTS package_plans;
DROP INDEX IF EXISTS idx_package_sites_plan;
DROP INDEX IF EXISTS idx_package_sites_site;
DROP INDEX IF EXISTS idx_package_sites_pkg;
DROP TABLE IF EXISTS package_sites;
DROP INDEX IF EXISTS idx_phase_packages_phase;
DROP TABLE IF EXISTS phase_packages;
DROP INDEX IF EXISTS idx_phase_sites_phase;
DROP TABLE IF EXISTS phase_sites;
-- SQLite 는 기존 표의 추가 컬럼(project_phases.weight 등, work_tasks.package_id/plan_id)을 DROP COLUMN 하지 않는다.
