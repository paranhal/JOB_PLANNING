-- 87-0 업무 그룹·구성원·일지·투입. 시드 데이터 없음.

CREATE TABLE IF NOT EXISTS work_groups (
  group_id    TEXT PRIMARY KEY,
  org_id      TEXT NOT NULL,
  name        TEXT NOT NULL,
  purpose     TEXT NOT NULL DEFAULT '',
  project_id  TEXT,
  status      TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','closed')),
  start_date  TEXT,
  end_date    TEXT,
  created_by  TEXT NOT NULL,
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  closed_by   TEXT,
  closed_at   DATETIME,
  CHECK (end_date IS NULL OR start_date IS NULL OR end_date >= start_date)
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_work_groups_name ON work_groups(org_id, name) WHERE status='active';
CREATE INDEX IF NOT EXISTS ix_work_groups_org ON work_groups(org_id, status);

CREATE TABLE IF NOT EXISTS work_group_members (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  group_id   TEXT NOT NULL REFERENCES work_groups(group_id),
  user_id    TEXT NOT NULL,
  role       TEXT NOT NULL CHECK (role IN ('leader','member')),
  joined_on  TEXT NOT NULL,
  left_on    TEXT,
  added_by   TEXT NOT NULL,
  removed_by TEXT,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK (left_on IS NULL OR left_on >= joined_on)
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_wgm_active_member ON work_group_members(group_id, user_id) WHERE left_on IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ux_wgm_active_leader ON work_group_members(group_id) WHERE role='leader' AND left_on IS NULL;
CREATE INDEX IF NOT EXISTS ix_wgm_user ON work_group_members(user_id, left_on);

CREATE TABLE IF NOT EXISTS work_group_events (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  group_id  TEXT NOT NULL,
  at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  actor_id  TEXT NOT NULL,
  action    TEXT NOT NULL,
  detail    TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS work_log_notes (
  log_id      TEXT PRIMARY KEY,
  org_id      TEXT NOT NULL,
  work_date   TEXT NOT NULL,
  project_id  TEXT,
  group_id    TEXT,
  scope       TEXT NOT NULL CHECK (scope IN ('group','individual')),
  author_id   TEXT NOT NULL,
  content     TEXT NOT NULL DEFAULT '',
  next_plan   TEXT NOT NULL DEFAULT '',
  status      TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','submitted')),
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK (scope='individual' OR group_id IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS ix_wln_date ON work_log_notes(org_id, work_date);
CREATE INDEX IF NOT EXISTS ix_wln_project ON work_log_notes(project_id, work_date);

CREATE TABLE IF NOT EXISTS work_effort_entries (
  entry_id    TEXT PRIMARY KEY,
  org_id      TEXT NOT NULL,
  work_date   TEXT NOT NULL,
  user_id     TEXT NOT NULL,
  project_id  TEXT,
  source_type TEXT NOT NULL DEFAULT 'manual',
  source_id   TEXT NOT NULL DEFAULT '',
  log_id      TEXT REFERENCES work_log_notes(log_id),
  group_id    TEXT,
  group_role  TEXT NOT NULL DEFAULT '' CHECK (group_role IN ('','leader','member')),
  role_label  TEXT NOT NULL DEFAULT '',
  minutes     INTEGER NOT NULL CHECK (minutes >= 0 AND minutes <= 1440),
  entry_mode  TEXT NOT NULL CHECK (entry_mode IN ('self','by_leader')),
  entered_by  TEXT NOT NULL,
  note        TEXT NOT NULL DEFAULT '',
  revision    INTEGER NOT NULL DEFAULT 1,
  voided_at   DATETIME,
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS ix_wee_project ON work_effort_entries(project_id, work_date);
CREATE INDEX IF NOT EXISTS ix_wee_user ON work_effort_entries(user_id, work_date);
CREATE INDEX IF NOT EXISTS ix_wee_group ON work_effort_entries(group_id, work_date);

CREATE TABLE IF NOT EXISTS work_effort_entry_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  entry_id TEXT NOT NULL,
  at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  actor_id TEXT NOT NULL,
  action TEXT NOT NULL,
  detail TEXT NOT NULL DEFAULT ''
);

CREATE TRIGGER IF NOT EXISTS trg_wge_no_update BEFORE UPDATE ON work_group_events
BEGIN
  SELECT RAISE(ABORT, 'work_group_events append-only');
END;
CREATE TRIGGER IF NOT EXISTS trg_wge_no_delete BEFORE DELETE ON work_group_events
BEGIN
  SELECT RAISE(ABORT, 'work_group_events append-only');
END;
CREATE TRIGGER IF NOT EXISTS trg_weel_no_update BEFORE UPDATE ON work_effort_entry_log
BEGIN
  SELECT RAISE(ABORT, 'work_effort_entry_log append-only');
END;
CREATE TRIGGER IF NOT EXISTS trg_weel_no_delete BEFORE DELETE ON work_effort_entry_log
BEGIN
  SELECT RAISE(ABORT, 'work_effort_entry_log append-only');
END;
CREATE TRIGGER IF NOT EXISTS trg_wg_no_delete BEFORE DELETE ON work_groups
BEGIN
  SELECT RAISE(ABORT, 'work_groups close-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_wgm_org_role BEFORE INSERT ON work_group_members
BEGIN
  SELECT RAISE(ABORT, 'member not allowed')
  WHERE EXISTS (
    SELECT 1 FROM users u
    WHERE u.user_id = NEW.user_id
      AND (
        TRIM(COALESCE(u.role,'')) IN ('c_level','observer','tester','partner')
        OR COALESCE(u.is_active,0) != 1
        OR TRIM(COALESCE(u.deleted_at,'')) != ''
      )
  );
  SELECT RAISE(ABORT, 'member org mismatch')
  WHERE EXISTS (
    SELECT 1 FROM work_groups g
    JOIN users u ON u.user_id = NEW.user_id
    WHERE g.group_id = NEW.group_id
      AND CASE WHEN TRIM(COALESCE(u.org_id,''))='' THEN 'O01' ELSE u.org_id END
        != g.org_id
  );
END;

CREATE TRIGGER IF NOT EXISTS trg_wee_org BEFORE INSERT ON work_effort_entries
BEGIN
  SELECT RAISE(ABORT, 'effort org mismatch')
  WHERE EXISTS (
    SELECT 1 FROM users u
    WHERE u.user_id = NEW.user_id
      AND CASE WHEN TRIM(COALESCE(u.org_id,''))='' THEN 'O01' ELSE u.org_id END
        != NEW.org_id
  );
END;
