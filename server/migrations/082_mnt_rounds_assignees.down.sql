DROP INDEX IF EXISTS idx_maintenance_visit_dedup;
DROP INDEX IF EXISTS ux_mnt_va_user;
DROP INDEX IF EXISTS ix_mnt_visit_assignees_visit;
DROP INDEX IF EXISTS ix_mnt_region_assignees_org;
DROP INDEX IF EXISTS ix_mnt_site_assignees_cust;
DROP TABLE IF EXISTS maintenance_visit_assignees;
DROP TABLE IF EXISTS maintenance_region_assignees;
DROP TABLE IF EXISTS maintenance_site_assignees;
CREATE UNIQUE INDEX IF NOT EXISTS idx_maintenance_visit_dedup
ON maintenance_visits(plan_id, visit_date, customer_id, ifnull(product_type,''));
-- SQLite는 컬럼 DROP이 불안정하므로 round_no·work_group_id·rounds_per_period 는 남긴다.
