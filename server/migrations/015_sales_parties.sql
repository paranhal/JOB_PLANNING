-- 015 영업 관계자·변경 이력 (§32.6~§32.7, 3단계)
-- work_projects 는 건드리지 않는다. 실제 적용은 InitDB → applySalesParties.
-- party_type · party_role 은 codes 에 둔다.

CREATE TABLE IF NOT EXISTS sales_parties (
  party_id         TEXT PRIMARY KEY,
  sales_id         TEXT NOT NULL,
  party_type       TEXT NOT NULL DEFAULT 'own',
  org_name         TEXT NOT NULL DEFAULT '',
  person_name      TEXT NOT NULL DEFAULT '',
  title            TEXT NOT NULL DEFAULT '',
  phone            TEXT NOT NULL DEFAULT '',
  email            TEXT NOT NULL DEFAULT '',
  party_role       TEXT NOT NULL DEFAULT '',
  is_primary       INTEGER NOT NULL DEFAULT 0,
  user_id          TEXT,
  contact_id       TEXT,
  is_active        INTEGER NOT NULL DEFAULT 1,
  replaced_by      TEXT,
  replaced_reason  TEXT NOT NULL DEFAULT '',
  note             TEXT NOT NULL DEFAULT '',
  created_at       DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sales_parties_sales ON sales_parties(sales_id, party_type, is_active);

CREATE TABLE IF NOT EXISTS sales_changes (
  change_id    TEXT PRIMARY KEY,
  sales_id     TEXT NOT NULL,
  field_key    TEXT NOT NULL,
  old_value    TEXT NOT NULL DEFAULT '',
  new_value    TEXT NOT NULL DEFAULT '',
  changed_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
  changed_by   TEXT NOT NULL DEFAULT '',
  note         TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_sales_changes_sales ON sales_changes(sales_id, changed_at);

INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active) VALUES
  ('SPT01','sales_party_type','own','당사',1,1),
  ('SPT02','sales_party_type','partner','협력사',2,1),
  ('SPT03','sales_party_type','customer','고객',3,1),
  ('SPR01','sales_party_role','decision','결정권자',1,1),
  ('SPR02','sales_party_role','working','실무',2,1),
  ('SPR03','sales_party_role','purchase','구매',3,1),
  ('SPR04','sales_party_role','tech','기술검토',4,1),
  ('SPR05','sales_party_role','sales','영업',5,1),
  ('SPR06','sales_party_role','other','기타',6,1);
