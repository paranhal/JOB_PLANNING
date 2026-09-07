-- 016 영업 승격 (§32.10, 5단계)
-- work_projects.sales_project_id 로 원본 영업 건을 잇는다.
-- 영업 전용 컬럼(예상 금액·단계 등)은 다시 넣지 않는다. 실제 적용은 InitDB → applySalesPromote.

ALTER TABLE work_projects ADD COLUMN sales_project_id TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_work_projects_sales
  ON work_projects(sales_project_id)
  WHERE sales_project_id IS NOT NULL AND TRIM(sales_project_id) != '';
