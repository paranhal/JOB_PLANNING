-- 95-8 AS 조치 구조화 부품 행. 시드 INSERT 없음. 단가·금액 컬럼 없음.

CREATE TABLE IF NOT EXISTS as_parts_used (
  part_id     TEXT PRIMARY KEY,
  as_id       TEXT NOT NULL,
  process_id  TEXT NOT NULL DEFAULT '',
  item_id     TEXT,
  part_name   TEXT NOT NULL,
  part_code   TEXT NOT NULL DEFAULT '',
  qty         INTEGER NOT NULL DEFAULT 1,
  sort_order  INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL DEFAULT '',
  updated_at  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS ix_as_parts_as ON as_parts_used(as_id);
CREATE INDEX IF NOT EXISTS ix_as_parts_process ON as_parts_used(process_id);
