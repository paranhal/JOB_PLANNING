-- 034 단품·납품 유형 (§36.4 · §36.5 v2.28)
-- 실제 적용은 InitDB → applySalesDealTypeV228.
-- 표를 나누지 않는다. sales_projects.deal_type 만 가른다.

ALTER TABLE sales_projects ADD COLUMN deal_type TEXT NOT NULL DEFAULT 'build';
ALTER TABLE sales_projects ADD COLUMN po_no TEXT NOT NULL DEFAULT '';
ALTER TABLE sales_projects ADD COLUMN delivered_at TEXT NOT NULL DEFAULT '';

UPDATE sales_projects SET deal_type='build' WHERE TRIM(COALESCE(deal_type,''))='';

INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active) VALUES
  ('SSS01','sales_supply_stage','inquiry','문의 접수',1,1),
  ('SSS02','sales_supply_stage','quoted','견적 제출',2,1),
  ('SSS03','sales_supply_stage','ordered','수주',3,1),
  ('SSS04','sales_supply_stage','delivered','납품 완료',4,1),
  ('SSS05','sales_supply_stage','dropped','취소·실주',5,1);

CREATE INDEX IF NOT EXISTS idx_sales_projects_deal_type ON sales_projects(deal_type);
