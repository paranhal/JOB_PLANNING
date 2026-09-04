-- 035 품목 마스터 (§36.6 v2.28)
-- 실제 적용은 InitDB → applySalesItems.
-- assets 는 시리얼 단위 설치 개체라 카탈로그가 아니다. 여기서 품명·규격·단가를 통일한다.

CREATE TABLE IF NOT EXISTS sales_items (
  item_id           TEXT PRIMARY KEY,
  item_kind         TEXT NOT NULL DEFAULT 'goods',
  category          TEXT NOT NULL DEFAULT '',
  name              TEXT NOT NULL,
  spec              TEXT NOT NULL DEFAULT '',
  model             TEXT NOT NULL DEFAULT '',
  manufacturer      TEXT NOT NULL DEFAULT '',
  unit              TEXT NOT NULL DEFAULT 'EA',
  gov_item_no       TEXT NOT NULL DEFAULT '',
  list_price        INTEGER NOT NULL DEFAULT 0,
  last_price        INTEGER NOT NULL DEFAULT 0,
  last_quoted_at    TEXT NOT NULL DEFAULT '',
  last_customer     TEXT NOT NULL DEFAULT '',
  default_supplier  TEXT NOT NULL DEFAULT '',
  is_active         INTEGER NOT NULL DEFAULT 1,
  needs_review      INTEGER NOT NULL DEFAULT 0,
  notes             TEXT NOT NULL DEFAULT '',
  created_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at        DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sales_items_kind ON sales_items(item_kind);
CREATE INDEX IF NOT EXISTS idx_sales_items_name ON sales_items(name);
CREATE INDEX IF NOT EXISTS idx_sales_items_active ON sales_items(is_active, needs_review);

INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active) VALUES
  ('SIK01','sales_item_kind','goods','물품',1,1),
  ('SIK02','sales_item_kind','service','AS·작업',2,1),
  ('SIK03','sales_item_kind','labor','개발 용역',3,1),
  ('SIK04','sales_item_kind','etc','기타',4,1),
  ('SIC01','sales_item_category','RFID장비','RFID장비',1,1),
  ('SIC02','sales_item_category','소모품','소모품',2,1),
  ('SIC03','sales_item_category','SW','SW',3,1),
  ('SIC04','sales_item_category','공사','공사',4,1),
  ('SIU01','sales_item_unit','EA','EA',1,1),
  ('SIU02','sales_item_unit','식','식',2,1),
  ('SIU03','sales_item_unit','대','대',3,1),
  ('SIU04','sales_item_unit','Box','Box',4,1),
  ('SIU05','sales_item_unit','인','인',5,1),
  ('SIU06','sales_item_unit','M/M','M/M',6,1);
