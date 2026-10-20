DROP INDEX IF EXISTS idx_maintenance_plans_org_year;
CREATE UNIQUE INDEX IF NOT EXISTS idx_maintenance_plans_year ON maintenance_plans(plan_year);
