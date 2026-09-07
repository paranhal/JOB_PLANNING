-- 025 롤백. 묶음만 해제한다. 접수 행은 남긴다.
-- SQLite 3.35+ 의 DROP COLUMN 을 쓴다.

DROP INDEX IF EXISTS idx_as_receipts_group;
ALTER TABLE as_receipts DROP COLUMN receipt_group_id;
