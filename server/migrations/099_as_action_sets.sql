-- 95-85 AS 조치 세트. 운영 행 UPDATE·백필 없음.

CREATE TABLE IF NOT EXISTS as_action_sets (
  set_id TEXT PRIMARY KEY,
  as_id TEXT NOT NULL,
  seq INTEGER NOT NULL,
  state TEXT NOT NULL DEFAULT 'draft',
  assigned_to TEXT,
  assigned_user_id TEXT,
  work_place TEXT,
  process_type TEXT,
  process_type_reason TEXT,
  visit_date TEXT,
  cause_cat1 TEXT,
  cause_cat1_custom TEXT,
  cause_cat2 TEXT,
  cause_cat3 TEXT,
  cause_detail TEXT,
  complete_datetime TEXT,
  work_date TEXT,
  start_time TEXT,
  duration_min INTEGER,
  end_time TEXT,
  leave_same INTEGER NOT NULL DEFAULT 1,
  leave_time TEXT,
  action_taken TEXT,
  action_na INTEGER,
  action_na_reason TEXT,
  result_code TEXT,
  result_extra TEXT,
  created_by TEXT,
  created_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
  UNIQUE (as_id, seq)
);
CREATE INDEX IF NOT EXISTS idx_as_action_sets_as ON as_action_sets(as_id);

ALTER TABLE attachments ADD COLUMN action_set_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_attach_action_set ON attachments(action_set_id);
