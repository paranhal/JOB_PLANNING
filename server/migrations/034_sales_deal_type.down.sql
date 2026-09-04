-- 034 롤백. 단품 단계 codes 를 비활성화한다.
-- deal_type/po_no/delivered_at 컬럼은 SQLite 에서 떼지 않는다. 값은 build 로 되돌린다.

UPDATE sales_projects SET deal_type='build' WHERE deal_type='supply';
UPDATE codes SET is_active=0 WHERE code_id IN ('SSS01','SSS02','SSS03','SSS04','SSS05');
