-- 기획서 §12.10.4
-- 장애원인·결론 서술. cause_type(통계 코드)과 합치지 않는다.
-- 실제 앱은 InitDB 시 repository.applyASReceiptsCauseReport 가 동일 ALTER 를 실행한다.

ALTER TABLE as_receipts ADD COLUMN cause_detail TEXT;
ALTER TABLE as_receipts ADD COLUMN conclusion TEXT;
