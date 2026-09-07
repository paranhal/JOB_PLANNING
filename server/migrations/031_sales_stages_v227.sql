-- 031 영업 단계 8→6 (§32.3 v2.27)
-- 실제 적용은 InitDB → applySalesStagesV227.
-- sales_stage_history 의 옛 코드는 그대로 둔다.

ALTER TABLE sales_projects ADD COLUMN legacy_stage TEXT NOT NULL DEFAULT '';
ALTER TABLE sales_projects ADD COLUMN won_at TEXT NOT NULL DEFAULT '';
ALTER TABLE sales_projects ADD COLUMN contracted_at TEXT NOT NULL DEFAULT '';

INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active) VALUES
  ('SST10','sales_stage','negotiation','협상',4,1),
  ('SSP10','sales_stage_prob','negotiation','70',4,1),
  ('SAT11','sales_activity_type','rfp','제안서제출(RFP)',8,1);

UPDATE codes SET code_name='담당자 접촉', sort_order=1, is_active=1 WHERE code_id='SST02';
UPDATE codes SET code_name='정보 입수', sort_order=2, is_active=1 WHERE code_id='SST01';
UPDATE codes SET code_name='제안 진행', sort_order=3, is_active=1 WHERE code_id='SST03';
UPDATE codes SET code_name='협상', sort_order=4, is_active=1 WHERE code_id='SST10';
UPDATE codes SET code_name='수주', sort_order=5, is_active=1 WHERE code_id='SST07';
UPDATE codes SET code_name='실주', sort_order=6, is_active=1 WHERE code_id='SST09';
UPDATE codes SET is_active=0 WHERE code_id IN ('SST04','SST05','SST06','SST08');

UPDATE codes SET code_name='10', sort_order=1, is_active=1 WHERE code_id='SSP02';
UPDATE codes SET code_name='25', sort_order=2, is_active=1 WHERE code_id='SSP01';
UPDATE codes SET code_name='40', sort_order=3, is_active=1 WHERE code_id='SSP03';
UPDATE codes SET code_name='70', sort_order=4, is_active=1 WHERE code_id='SSP10';
UPDATE codes SET code_name='100', sort_order=5, is_active=1 WHERE code_id='SSP07';
UPDATE codes SET code_name='0', sort_order=6, is_active=1 WHERE code_id='SSP09';
UPDATE codes SET is_active=0 WHERE code_id IN ('SSP04','SSP05','SSP06','SSP08');

UPDATE sales_projects SET legacy_stage = stage WHERE TRIM(COALESCE(legacy_stage,'')) = '';

UPDATE sales_projects SET stage='proposal' WHERE stage IN ('quote','rfp','submit');
UPDATE sales_projects SET stage='won' WHERE stage='contracted';

UPDATE sales_projects SET contracted_at = COALESCE((
  SELECT date(MAX(h.changed_at)) FROM sales_stage_history h
  WHERE h.sales_id = sales_projects.sales_id AND h.to_stage='contracted'
), date(updated_at), date(created_at), '')
WHERE TRIM(COALESCE(legacy_stage,''))='contracted'
  AND TRIM(COALESCE(contracted_at,''))='';

UPDATE sales_projects SET won_at = COALESCE((
  SELECT date(MAX(h.changed_at)) FROM sales_stage_history h
  WHERE h.sales_id = sales_projects.sales_id AND h.to_stage IN ('won','contracted')
), date(updated_at), date(created_at), '')
WHERE stage='won' AND TRIM(COALESCE(won_at,''))='';

UPDATE sales_projects SET probability = CASE stage
  WHEN 'contact' THEN 10
  WHEN 'lead' THEN 25
  WHEN 'proposal' THEN 40
  WHEN 'negotiation' THEN 70
  WHEN 'won' THEN 100
  WHEN 'lost' THEN 0
  ELSE probability
END
WHERE probability_override IS NULL;
