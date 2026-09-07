-- 기획서 §34.2.1
-- 같은 날 같은 기관에서 함께 접수한 건을 묶는다.
-- 실제 앱은 InitDB 시 repository.applyASReceiptGroup 가 동일 ALTER 를 실행한다.

ALTER TABLE as_receipts ADD COLUMN receipt_group_id TEXT;
CREATE INDEX IF NOT EXISTS idx_as_receipts_group ON as_receipts(receipt_group_id);
