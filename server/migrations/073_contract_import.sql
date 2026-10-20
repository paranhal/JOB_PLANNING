-- 95-2 사업(계약) 엑셀 업로드. 시드 INSERT 없음.

ALTER TABLE work_projects ADD COLUMN direction TEXT NOT NULL DEFAULT 'revenue';

CREATE TABLE IF NOT EXISTS contract_end_customers (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  contract_id TEXT NOT NULL,
  customer_id TEXT NOT NULL,
  sort_order  INTEGER NOT NULL DEFAULT 0,
  UNIQUE(contract_id, customer_id)
);
CREATE INDEX IF NOT EXISTS ix_cec_contract ON contract_end_customers(contract_id);

CREATE TABLE IF NOT EXISTS contract_import_batches (
  id          TEXT PRIMARY KEY,
  file_name   TEXT NOT NULL,
  rows_total  INTEGER NOT NULL DEFAULT 0,
  rows_ok     INTEGER NOT NULL DEFAULT 0,
  rows_error  INTEGER NOT NULL DEFAULT 0,
  status      TEXT NOT NULL DEFAULT 'preview',
  created_by  TEXT,
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS ix_cib_created ON contract_import_batches(created_at);
