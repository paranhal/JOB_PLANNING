-- 040 롤백. SQLite 는 DROP COLUMN 을 버전마다 다르게 지원하므로
-- 코드 값은 비활성만 하고, 컬럼은 남겨 둔 채 값을 비운다.

UPDATE as_receipts SET visit_date = NULL, process_type_reason = NULL;
UPDATE codes SET is_active=0 WHERE code_id='PRT006';
