-- 95-175 A. 하부 업무·작업 계획. 95-120P project_phases 를 확장한다.
ALTER TABLE project_phases ADD COLUMN unit_kind TEXT NOT NULL DEFAULT 'other';
ALTER TABLE project_phases ADD COLUMN progress_mode TEXT NOT NULL DEFAULT 'site_count';
ALTER TABLE project_phases ADD COLUMN weight REAL NOT NULL DEFAULT 1;

CREATE TABLE IF NOT EXISTS phase_sites (
  phase_site_id TEXT PRIMARY KEY,
  phase_id TEXT NOT NULL,
  project_site_id TEXT NOT NULL,
  scope_status TEXT NOT NULL DEFAULT 'active',
  added_at TEXT NOT NULL,
  added_by TEXT NOT NULL DEFAULT '',
  removed_at TEXT NOT NULL DEFAULT '',
  removed_by TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL DEFAULT '',
  UNIQUE(phase_id, project_site_id)
);
CREATE INDEX IF NOT EXISTS idx_phase_sites_phase ON phase_sites(phase_id, scope_status);

CREATE TABLE IF NOT EXISTS phase_packages (
  package_id TEXT PRIMARY KEY,
  phase_id TEXT NOT NULL,
  seq INTEGER NOT NULL DEFAULT 0,
  name TEXT NOT NULL,
  pkg_kind TEXT NOT NULL DEFAULT 'other',
  track_mode TEXT NOT NULL DEFAULT 'site',
  follow_scope INTEGER NOT NULL DEFAULT 1,
  qty_unit TEXT NOT NULL DEFAULT '',
  qty_total INTEGER NOT NULL DEFAULT 0,
  weight REAL NOT NULL DEFAULT 1,
  track_progress INTEGER NOT NULL DEFAULT 1,
  status TEXT NOT NULL DEFAULT 'auto',
  done_at TEXT NOT NULL DEFAULT '',
  done_by TEXT NOT NULL DEFAULT '',
  next_note TEXT NOT NULL DEFAULT '',
  is_active INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(phase_id, name)
);
CREATE INDEX IF NOT EXISTS idx_phase_packages_phase ON phase_packages(phase_id, seq);

CREATE TABLE IF NOT EXISTS package_sites (
  package_site_id TEXT PRIMARY KEY,
  package_id TEXT NOT NULL,
  project_site_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'planned',
  target_qty INTEGER NOT NULL DEFAULT 0,
  done_qty INTEGER NOT NULL DEFAULT 0,
  plan_id TEXT NOT NULL DEFAULT '',
  exclude_reason TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL,
  UNIQUE(package_id, project_site_id)
);
CREATE INDEX IF NOT EXISTS idx_package_sites_pkg ON package_sites(package_id, status);
CREATE INDEX IF NOT EXISTS idx_package_sites_site ON package_sites(project_site_id);
CREATE INDEX IF NOT EXISTS idx_package_sites_plan ON package_sites(plan_id);

CREATE TABLE IF NOT EXISTS package_plans (
  plan_id TEXT PRIMARY KEY,
  package_id TEXT NOT NULL,
  seq INTEGER NOT NULL DEFAULT 0,
  name TEXT NOT NULL,
  plan_start TEXT NOT NULL DEFAULT '',
  plan_end TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'auto',
  progress_basis TEXT NOT NULL DEFAULT 'site',
  qty_unit TEXT NOT NULL DEFAULT '',
  region_label TEXT NOT NULL DEFAULT '',
  next_note TEXT NOT NULL DEFAULT '',
  is_active INTEGER NOT NULL DEFAULT 1,
  done_at TEXT NOT NULL DEFAULT '',
  done_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(package_id, name)
);
CREATE INDEX IF NOT EXISTS idx_package_plans_pkg ON package_plans(package_id, seq);
