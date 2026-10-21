-- 95-3 학교 마스터 기관코드 · 사업 사이트 설치/검수 진행. 시드 INSERT 없음.
-- project_sites.status CHECK 재작성(active/done → in_progress/inspected)은 Go apply 가 한 번만 한다.

ALTER TABLE customers ADD COLUMN org_code TEXT NOT NULL DEFAULT '';

ALTER TABLE project_sites ADD COLUMN installed_at TEXT NOT NULL DEFAULT '';
ALTER TABLE project_sites ADD COLUMN inspected_at TEXT NOT NULL DEFAULT '';
ALTER TABLE project_sites ADD COLUMN excluded_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE project_sites ADD COLUMN status_changed_at TEXT NOT NULL DEFAULT '';
ALTER TABLE project_sites ADD COLUMN status_changed_by TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS ix_cust_org_code
  ON customers(org_id, parent_customer_id, org_code)
  WHERE org_code IS NOT NULL AND TRIM(org_code) != '';

CREATE UNIQUE INDEX IF NOT EXISTS ix_ps_project_customer
  ON project_sites(project_id, customer_id)
  WHERE customer_id IS NOT NULL AND TRIM(customer_id) != '';

CREATE TABLE IF NOT EXISTS project_site_events (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  site_id      TEXT NOT NULL,
  project_id   TEXT NOT NULL,
  from_status  TEXT NOT NULL DEFAULT '',
  to_status    TEXT NOT NULL,
  event_date   TEXT NOT NULL DEFAULT '',
  note         TEXT NOT NULL DEFAULT '',
  created_by   TEXT,
  created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  source       TEXT NOT NULL DEFAULT 'ui' CHECK (source IN ('ui','excel'))
);
CREATE INDEX IF NOT EXISTS ix_pse_site ON project_site_events(site_id, created_at);
CREATE INDEX IF NOT EXISTS ix_pse_project ON project_site_events(project_id, created_at);

CREATE TABLE IF NOT EXISTS site_import_batches (
  id          TEXT PRIMARY KEY,
  project_id  TEXT NOT NULL,
  file_name   TEXT NOT NULL,
  mode        TEXT NOT NULL DEFAULT 'add',
  rows_total  INTEGER NOT NULL DEFAULT 0,
  rows_ok     INTEGER NOT NULL DEFAULT 0,
  rows_error  INTEGER NOT NULL DEFAULT 0,
  status      TEXT NOT NULL DEFAULT 'preview',
  created_by  TEXT,
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS ix_sib_project ON site_import_batches(project_id, created_at);
