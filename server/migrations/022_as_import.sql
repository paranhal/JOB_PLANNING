-- 022 AS 과거 데이터 정제 반입 (§12.11.7)
-- 업로드만으로는 접수에 넣지 않는다. 검증 리포트 → 확인(제외) → 반영.
-- 반영 건은 data_origin='import' + import_batch_id. 배치로 일괄 취소한다.

CREATE TABLE IF NOT EXISTS as_import_batches (
  batch_id      TEXT PRIMARY KEY,
  filename      TEXT NOT NULL DEFAULT '',
  status        TEXT NOT NULL DEFAULT 'reported',
  created_at    TEXT NOT NULL DEFAULT '',
  created_by    TEXT NOT NULL DEFAULT '',
  applied_at    TEXT NOT NULL DEFAULT '',
  cancelled_at  TEXT NOT NULL DEFAULT '',
  total_rows    INTEGER NOT NULL DEFAULT 0,
  error_rows    INTEGER NOT NULL DEFAULT 0,
  warning_rows  INTEGER NOT NULL DEFAULT 0,
  applied_rows  INTEGER NOT NULL DEFAULT 0,
  note          TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS as_import_rows (
  batch_id      TEXT NOT NULL,
  row_no        INTEGER NOT NULL,
  excluded      INTEGER NOT NULL DEFAULT 0,
  issues        TEXT NOT NULL DEFAULT '',
  customer_raw  TEXT NOT NULL DEFAULT '',
  customer_id   TEXT NOT NULL DEFAULT '',
  asset_raw     TEXT NOT NULL DEFAULT '',
  asset_id      TEXT NOT NULL DEFAULT '',
  receipt_raw   TEXT NOT NULL DEFAULT '',
  receipt_date  TEXT NOT NULL DEFAULT '',
  process_raw   TEXT NOT NULL DEFAULT '',
  process_date  TEXT NOT NULL DEFAULT '',
  visit_date    TEXT NOT NULL DEFAULT '',
  symptom       TEXT NOT NULL DEFAULT '',
  action        TEXT NOT NULL DEFAULT '',
  channel       TEXT NOT NULL DEFAULT '',
  requester     TEXT NOT NULL DEFAULT '',
  worker        TEXT NOT NULL DEFAULT '',
  worker_uid    TEXT NOT NULL DEFAULT '',
  category      TEXT NOT NULL DEFAULT '',
  model         TEXT NOT NULL DEFAULT '',
  applied_as_id TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (batch_id, row_no)
);

ALTER TABLE as_receipts ADD COLUMN import_batch_id TEXT;
CREATE INDEX IF NOT EXISTS idx_as_receipts_import_batch ON as_receipts(import_batch_id);
