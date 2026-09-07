-- 기획서 §34.3.5 이관후속 「추가 접수 내용」
-- 실제 앱은 InitDB 시 repository.applyAS34TransferFollowup 가 동일 내용을 실행한다.

ALTER TABLE as_receipts ADD COLUMN followup_note TEXT;
