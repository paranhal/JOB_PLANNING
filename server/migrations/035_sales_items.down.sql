-- 035 롤백. 품목 마스터와 시드 codes 를 뗀다.

DROP INDEX IF EXISTS idx_sales_items_active;
DROP INDEX IF EXISTS idx_sales_items_name;
DROP INDEX IF EXISTS idx_sales_items_kind;
DROP TABLE IF EXISTS sales_items;

UPDATE codes SET is_active=0 WHERE code_id IN (
  'SIK01','SIK02','SIK03','SIK04',
  'SIC01','SIC02','SIC03','SIC04',
  'SIU01','SIU02','SIU03','SIU04','SIU05','SIU06'
);
