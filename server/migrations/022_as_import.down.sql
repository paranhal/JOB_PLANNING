-- 022 롤백. 반입 스테이징만 지운다. 이미 반영된 접수는 배치 취소로 되돌린다.

DROP INDEX IF EXISTS idx_as_receipts_import_batch;
DROP TABLE IF EXISTS as_import_rows;
DROP TABLE IF EXISTS as_import_batches;
DELETE FROM id_sequences WHERE seq_key='as_import_batch';
