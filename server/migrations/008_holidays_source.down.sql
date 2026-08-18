-- 008 롤백. 시드 행은 남기고 추가 컬럼만 제거한다.
-- SQLite 3.35+ 의 DROP COLUMN 을 쓴다.

ALTER TABLE holidays DROP COLUMN source;
ALTER TABLE holidays DROP COLUMN synced_at;
