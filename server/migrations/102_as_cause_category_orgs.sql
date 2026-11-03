-- 95-97 조직별 원인분류표. 기존 as_cause_categories 는 그대로 둔다.

CREATE TABLE IF NOT EXISTS as_cause_category_orgs (
  org_id TEXT NOT NULL,
  code TEXT NOT NULL,
  level INTEGER NOT NULL,
  parent_code TEXT NOT NULL DEFAULT '',
  label TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0,
  is_active INTEGER NOT NULL DEFAULT 1,
  cause_type_map TEXT NOT NULL DEFAULT '',
  is_fault INTEGER NOT NULL DEFAULT 1,
  created_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
  PRIMARY KEY (org_id, code),
  UNIQUE (org_id, level, parent_code, label)
);
CREATE INDEX IF NOT EXISTS idx_as_cause_cat_orgs_parent ON as_cause_category_orgs(org_id, parent_code, sort_order);
