-- 95-176. 수행 구조 템플릿. 전사 공통이라 이름만 유일하고, org_id 는 만든 사업의 조직 기록이다.
-- 계획·사이트·날짜·상태는 두지 않는다. 기존 사업 행은 바꾸지 않는다.

CREATE TABLE IF NOT EXISTS structure_templates (
  template_id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  project_kind TEXT NOT NULL DEFAULT '',
  note TEXT NOT NULL DEFAULT '',
  is_active INTEGER NOT NULL DEFAULT 1,
  created_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_structure_templates_name ON structure_templates(name);

CREATE TABLE IF NOT EXISTS structure_template_phases (
  tphase_id TEXT PRIMARY KEY,
  template_id TEXT NOT NULL,
  seq INTEGER NOT NULL DEFAULT 0,
  name TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_structure_tpl_phases ON structure_template_phases(template_id, seq);

CREATE TABLE IF NOT EXISTS structure_template_packages (
  tpackage_id TEXT PRIMARY KEY,
  tphase_id TEXT NOT NULL,
  seq INTEGER NOT NULL DEFAULT 0,
  name TEXT NOT NULL,
  track_mode TEXT NOT NULL DEFAULT 'site',
  weight REAL NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_structure_tpl_pkgs ON structure_template_packages(tphase_id, seq);
