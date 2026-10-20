-- 87 팀 간 업무 협조. 시드 데이터 없음.

CREATE TABLE IF NOT EXISTS work_cooperations (
  coop_id           TEXT PRIMARY KEY,
  req_org_id        TEXT NOT NULL,
  req_user_id       TEXT NOT NULL,
  target_org_id     TEXT NOT NULL,
  kind              TEXT NOT NULL CHECK (kind IN ('as','inspection','admin','project')),
  title             TEXT NOT NULL,
  body              TEXT NOT NULL DEFAULT '',
  project_id        TEXT,
  want_start        TEXT,
  want_end          TEXT,
  status            TEXT NOT NULL DEFAULT 'requested' CHECK (status IN ('requested','accepted','assigned','completed','rejected','cancelled')),
  reject_reason     TEXT,
  plan_start        TEXT,
  plan_end          TEXT,
  instruction       TEXT NOT NULL DEFAULT '',
  work_group_id     TEXT,
  decided_by        TEXT,
  decided_at        DATETIME,
  completed_reason  TEXT,
  created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK (req_org_id <> target_org_id),
  CHECK (want_end IS NULL OR want_start IS NULL OR want_end >= want_start),
  CHECK (status <> 'rejected' OR (reject_reason IS NOT NULL AND TRIM(reject_reason) <> ''))
);
CREATE INDEX IF NOT EXISTS ix_coop_req ON work_cooperations(req_org_id, status);
CREATE INDEX IF NOT EXISTS ix_coop_target ON work_cooperations(target_org_id, status);
CREATE INDEX IF NOT EXISTS ix_coop_user ON work_cooperations(req_user_id);

CREATE TABLE IF NOT EXISTS cooperation_files (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  cooperation_id TEXT NOT NULL,
  file_name      TEXT NOT NULL,
  stored_path    TEXT NOT NULL,
  mime           TEXT NOT NULL DEFAULT '',
  size           INTEGER NOT NULL DEFAULT 0,
  uploaded_by    TEXT NOT NULL,
  created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS ix_coop_files ON cooperation_files(cooperation_id);

CREATE TABLE IF NOT EXISTS cooperation_assignees (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  cooperation_id TEXT NOT NULL,
  user_id        TEXT NOT NULL,
  role           TEXT NOT NULL CHECK (role IN ('lead','member')),
  work_group_id  TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_coop_assignee ON cooperation_assignees(cooperation_id, user_id);
CREATE INDEX IF NOT EXISTS ix_coop_assignee_user ON cooperation_assignees(user_id);

CREATE TABLE IF NOT EXISTS cooperation_events (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  cooperation_id TEXT NOT NULL,
  at             DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  actor_id       TEXT NOT NULL,
  action         TEXT NOT NULL,
  detail         TEXT NOT NULL DEFAULT '',
  on_behalf      TEXT
);

CREATE TRIGGER IF NOT EXISTS trg_coop_ev_no_update BEFORE UPDATE ON cooperation_events
BEGIN
  SELECT RAISE(ABORT, 'cooperation_events append-only');
END;
CREATE TRIGGER IF NOT EXISTS trg_coop_ev_no_delete BEFORE DELETE ON cooperation_events
BEGIN
  SELECT RAISE(ABORT, 'cooperation_events append-only');
END;

ALTER TABLE work_tasks ADD COLUMN cooperation_id TEXT;
ALTER TABLE as_receipts ADD COLUMN cooperation_id TEXT;
ALTER TABLE as_receipts ADD COLUMN coop_incomplete INTEGER NOT NULL DEFAULT 0;
ALTER TABLE maintenance_visits ADD COLUMN cooperation_id TEXT;
CREATE INDEX IF NOT EXISTS ix_task_coop ON work_tasks(cooperation_id);
CREATE INDEX IF NOT EXISTS ix_as_coop ON as_receipts(cooperation_id);
CREATE INDEX IF NOT EXISTS ix_mnt_coop ON maintenance_visits(cooperation_id);
