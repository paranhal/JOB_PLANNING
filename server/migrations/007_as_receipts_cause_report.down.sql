-- 007 롤백. 서술 컬럼만 제거한다. cause_type 은 그대로 둔다.
-- SQLite 3.35+ 의 DROP COLUMN 을 쓴다.

ALTER TABLE as_receipts DROP COLUMN cause_detail;
ALTER TABLE as_receipts DROP COLUMN conclusion;
