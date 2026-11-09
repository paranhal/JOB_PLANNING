-- 95-161A 연차 원장(회계연도) · 결근 전환. 부여·소진·결근 기록은 append-only.

ALTER TABLE employee_profiles ADD COLUMN leave_auto_enabled INTEGER NOT NULL DEFAULT 0;

ALTER TABLE leave_requests ADD COLUMN absence_consent INTEGER NOT NULL DEFAULT 0;
ALTER TABLE leave_requests ADD COLUMN deduct_minutes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE leave_requests ADD COLUMN absence_minutes INTEGER NOT NULL DEFAULT 0;

INSERT OR IGNORE INTO leave_types(type_key,label,unit,deducts_balance,reflect_staff_leaves,sort_order) VALUES
 ('military_training','예비군(공가)','day',0,0,90);

CREATE TABLE IF NOT EXISTS leave_grants (
  grant_id   TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL,
  grant_key  TEXT NOT NULL,
  minutes    INTEGER NOT NULL,
  granted_on TEXT NOT NULL,
  expires_on TEXT NOT NULL,
  source     TEXT NOT NULL,
  note       TEXT,
  created_by TEXT,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (user_id, grant_key)
);
CREATE INDEX IF NOT EXISTS idx_leave_grants_user ON leave_grants(user_id, granted_on);

CREATE TABLE IF NOT EXISTS leave_consumptions (
  cons_id      TEXT PRIMARY KEY,
  user_id      TEXT NOT NULL,
  grant_id     TEXT NOT NULL,
  leave_req_id TEXT,
  minutes      INTEGER NOT NULL,
  kind         TEXT NOT NULL,
  created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_leave_cons_grant ON leave_consumptions(grant_id);
CREATE INDEX IF NOT EXISTS idx_leave_cons_req ON leave_consumptions(leave_req_id);

CREATE TABLE IF NOT EXISTS leave_absence_records (
  absence_id   TEXT PRIMARY KEY,
  user_id      TEXT NOT NULL,
  leave_req_id TEXT,
  minutes      INTEGER NOT NULL,
  kind         TEXT NOT NULL,
  note         TEXT,
  created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_leave_abs_req ON leave_absence_records(leave_req_id);

CREATE TRIGGER IF NOT EXISTS trg_leave_grants_no_upd BEFORE UPDATE ON leave_grants
BEGIN SELECT RAISE(ABORT, 'leave_grants append-only'); END;

CREATE TRIGGER IF NOT EXISTS trg_leave_grants_no_del BEFORE DELETE ON leave_grants
BEGIN SELECT RAISE(ABORT, 'leave_grants append-only'); END;

CREATE TRIGGER IF NOT EXISTS trg_leave_cons_no_upd BEFORE UPDATE ON leave_consumptions
BEGIN SELECT RAISE(ABORT, 'leave_consumptions append-only'); END;

CREATE TRIGGER IF NOT EXISTS trg_leave_cons_no_del BEFORE DELETE ON leave_consumptions
BEGIN SELECT RAISE(ABORT, 'leave_consumptions append-only'); END;

CREATE TRIGGER IF NOT EXISTS trg_leave_abs_no_upd BEFORE UPDATE ON leave_absence_records
BEGIN SELECT RAISE(ABORT, 'leave_absence_records append-only'); END;

CREATE TRIGGER IF NOT EXISTS trg_leave_abs_no_del BEFORE DELETE ON leave_absence_records
BEGIN SELECT RAISE(ABORT, 'leave_absence_records append-only'); END;

CREATE TRIGGER IF NOT EXISTS trg_leave_cons_cap BEFORE INSERT ON leave_consumptions
WHEN NEW.minutes > 0 AND (
  SELECT COALESCE(SUM(minutes),0) FROM leave_consumptions WHERE grant_id=NEW.grant_id
) + NEW.minutes > (SELECT minutes FROM leave_grants WHERE grant_id=NEW.grant_id)
BEGIN SELECT RAISE(ABORT, 'over-consume'); END;
