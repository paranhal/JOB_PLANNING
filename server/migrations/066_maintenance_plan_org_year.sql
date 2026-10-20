-- 정기점검 계획은 조직+연도로 유일하다. 행의 org_id 는 고치지 않는다.

DROP INDEX IF EXISTS idx_maintenance_plans_year;
CREATE UNIQUE INDEX IF NOT EXISTS idx_maintenance_plans_org_year ON maintenance_plans(org_id, plan_year);
