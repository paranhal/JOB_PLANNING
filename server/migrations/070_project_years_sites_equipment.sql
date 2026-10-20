-- 95 다년 사업 연차·사이트·장비. 시드 분류 없음. 85 단계·기성·서류 표가 아직 없어 year_id 는 붙이지 않는다.

CREATE TABLE IF NOT EXISTS contract_years (
  id          TEXT PRIMARY KEY,
  project_id  TEXT NOT NULL REFERENCES work_projects(project_id),
  year_no     INTEGER NOT NULL,
  label       TEXT NOT NULL DEFAULT '',
  start_date  TEXT NOT NULL,
  end_date    TEXT NOT NULL,
  amount      INTEGER NOT NULL DEFAULT 0,
  status      TEXT NOT NULL DEFAULT 'planned' CHECK (status IN ('planned','active','done','cancelled')),
  note        TEXT NOT NULL DEFAULT '',
  org_id      TEXT NOT NULL,
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by  TEXT,
  CHECK (end_date >= start_date),
  CHECK (year_no >= 1),
  UNIQUE (project_id, year_no)
);
CREATE INDEX IF NOT EXISTS ix_cy_project ON contract_years(project_id, year_no);
CREATE INDEX IF NOT EXISTS ix_cy_org ON contract_years(org_id);

CREATE TABLE IF NOT EXISTS project_sites (
  id            TEXT PRIMARY KEY,
  project_id    TEXT NOT NULL REFERENCES work_projects(project_id),
  customer_id   TEXT,
  site_name     TEXT NOT NULL,
  region        TEXT NOT NULL DEFAULT '',
  address       TEXT NOT NULL DEFAULT '',
  contact_note  TEXT NOT NULL DEFAULT '',
  status        TEXT NOT NULL DEFAULT 'planned' CHECK (status IN ('planned','active','done','excluded')),
  sort_order    INTEGER NOT NULL DEFAULT 0,
  is_active     INTEGER NOT NULL DEFAULT 1,
  note          TEXT NOT NULL DEFAULT '',
  org_id        TEXT NOT NULL,
  UNIQUE (project_id, site_name)
);
CREATE INDEX IF NOT EXISTS ix_ps_project ON project_sites(project_id, is_active, sort_order);
CREATE INDEX IF NOT EXISTS ix_ps_org ON project_sites(org_id);
CREATE INDEX IF NOT EXISTS ix_ps_region ON project_sites(project_id, region);

CREATE TABLE IF NOT EXISTS equipment_categories (
  id         TEXT PRIMARY KEY,
  org_id     TEXT NOT NULL,
  parent_id  TEXT,
  code       TEXT NOT NULL,
  name       TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0,
  is_active  INTEGER NOT NULL DEFAULT 1,
  UNIQUE (org_id, code)
);
CREATE INDEX IF NOT EXISTS ix_ec_org ON equipment_categories(org_id, is_active, sort_order);

CREATE TABLE IF NOT EXISTS equipment_category_fields (
  id          TEXT PRIMARY KEY,
  category_id TEXT NOT NULL REFERENCES equipment_categories(id),
  field_key   TEXT NOT NULL,
  label       TEXT NOT NULL,
  field_type  TEXT NOT NULL CHECK (field_type IN ('text','number','date','select')),
  options     TEXT NOT NULL DEFAULT '',
  required    INTEGER NOT NULL DEFAULT 0,
  sort_order  INTEGER NOT NULL DEFAULT 0,
  is_active   INTEGER NOT NULL DEFAULT 1,
  UNIQUE (category_id, field_key)
);

CREATE TABLE IF NOT EXISTS project_equipment (
  id               TEXT PRIMARY KEY,
  project_id       TEXT NOT NULL REFERENCES work_projects(project_id),
  site_id          TEXT NOT NULL REFERENCES project_sites(id),
  year_id          TEXT REFERENCES contract_years(id),
  category_id      TEXT,
  product_name     TEXT NOT NULL,
  model            TEXT NOT NULL DEFAULT '',
  manufacturer     TEXT NOT NULL DEFAULT '',
  qty              INTEGER NOT NULL DEFAULT 1 CHECK (qty >= 1),
  serial_no        TEXT,
  install_location TEXT NOT NULL DEFAULT '',
  install_date     TEXT,
  status           TEXT NOT NULL DEFAULT 'planned' CHECK (status IN ('planned','delivered','installed','accepted','replaced','removed')),
  asset_id         TEXT,
  note             TEXT NOT NULL DEFAULT '',
  org_id           TEXT NOT NULL,
  is_active        INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS ix_pe_site ON project_equipment(project_id, site_id);
CREATE INDEX IF NOT EXISTS ix_pe_cat ON project_equipment(project_id, category_id);
CREATE INDEX IF NOT EXISTS ix_pe_status ON project_equipment(project_id, status);
CREATE INDEX IF NOT EXISTS ix_pe_serial ON project_equipment(serial_no) WHERE serial_no IS NOT NULL AND TRIM(serial_no) != '';

CREATE TABLE IF NOT EXISTS project_equipment_values (
  equipment_id TEXT NOT NULL REFERENCES project_equipment(id),
  field_id     TEXT NOT NULL REFERENCES equipment_category_fields(id),
  value        TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (equipment_id, field_id)
);

CREATE TABLE IF NOT EXISTS equipment_import_batches (
  id          TEXT PRIMARY KEY,
  project_id  TEXT NOT NULL,
  file_name   TEXT NOT NULL,
  rows_total  INTEGER NOT NULL DEFAULT 0,
  rows_ok     INTEGER NOT NULL DEFAULT 0,
  rows_error  INTEGER NOT NULL DEFAULT 0,
  status      TEXT NOT NULL DEFAULT 'preview',
  created_by  TEXT,
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TRIGGER IF NOT EXISTS trg_cy_overlap_ins BEFORE INSERT ON contract_years
BEGIN
  SELECT RAISE(ABORT, 'year overlap')
  WHERE NEW.status != 'cancelled'
    AND EXISTS (
      SELECT 1 FROM contract_years y
      WHERE y.project_id = NEW.project_id
        AND y.status != 'cancelled'
        AND y.start_date <= NEW.end_date
        AND NEW.start_date <= y.end_date
    );
END;

CREATE TRIGGER IF NOT EXISTS trg_cy_overlap_upd BEFORE UPDATE ON contract_years
BEGIN
  SELECT RAISE(ABORT, 'year overlap')
  WHERE NEW.status != 'cancelled'
    AND EXISTS (
      SELECT 1 FROM contract_years y
      WHERE y.project_id = NEW.project_id
        AND y.id != NEW.id
        AND y.status != 'cancelled'
        AND y.start_date <= NEW.end_date
        AND NEW.start_date <= y.end_date
    );
END;
