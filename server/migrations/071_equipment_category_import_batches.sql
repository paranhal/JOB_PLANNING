-- 95-1 장비 분류표 엑셀 업로드 배치. 원본 파일은 저장하지 않는다. 시드 분류 없음.

CREATE TABLE IF NOT EXISTS equipment_category_import_batches (
  id          TEXT PRIMARY KEY,
  org_scope   TEXT NOT NULL DEFAULT '',
  file_name   TEXT NOT NULL,
  rows_total  INTEGER NOT NULL DEFAULT 0,
  rows_ok     INTEGER NOT NULL DEFAULT 0,
  rows_error  INTEGER NOT NULL DEFAULT 0,
  status      TEXT NOT NULL DEFAULT 'preview',
  created_by  TEXT,
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS ix_ecib_created ON equipment_category_import_batches(created_at);
