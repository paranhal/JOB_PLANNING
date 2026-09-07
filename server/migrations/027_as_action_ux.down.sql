-- 롤백 §34.3. 원인분류 컬럼·표만 되돌린다. process_type 이관은 되돌리지 않는다.

DROP TABLE IF EXISTS as_cause_categories;
ALTER TABLE as_receipts DROP COLUMN cause_cat1;
ALTER TABLE as_receipts DROP COLUMN cause_cat2;
ALTER TABLE as_receipts DROP COLUMN cause_cat3;

UPDATE codes SET is_active=1
 WHERE code_group='process_type' AND code_value IN ('replace','config');
UPDATE codes SET is_active=1
 WHERE code_group='result_code' AND code_value IN ('revisit_needed','temporary','escalation');
