package repository

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func applyLeaveRequestsV72(db *sql.DB) {
	if db == nil {
		return
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS leave_policies (
  org_id               TEXT PRIMARY KEY,
  work_minutes_per_day INTEGER NOT NULL DEFAULT 480,
  allow_negative       INTEGER NOT NULL DEFAULT 0,
  doc_prefix           TEXT NOT NULL DEFAULT 'VI',
  ledger_start_date    TEXT,
  half_am_start        TEXT NOT NULL DEFAULT '09:00',
  half_am_end          TEXT NOT NULL DEFAULT '13:00',
  half_pm_start        TEXT NOT NULL DEFAULT '14:00',
  half_pm_end          TEXT NOT NULL DEFAULT '18:00',
  updated_at           DATETIME DEFAULT CURRENT_TIMESTAMP
)`,
		`CREATE TABLE IF NOT EXISTS leave_types (
  type_key             TEXT PRIMARY KEY,
  label                TEXT NOT NULL,
  unit                 TEXT NOT NULL,
  deducts_balance      INTEGER NOT NULL,
  reflect_staff_leaves INTEGER NOT NULL,
  is_active            INTEGER NOT NULL DEFAULT 1,
  sort_order           INTEGER NOT NULL DEFAULT 0
)`,
		`INSERT OR IGNORE INTO leave_types(type_key,label,unit,deducts_balance,reflect_staff_leaves,sort_order) VALUES
 ('annual','연차','day',1,1,10),
 ('half','반차','time',1,1,20),
 ('monthly','월차','day',1,1,30),
 ('vacation','휴가','day',1,1,40),
 ('early_leave','조퇴','time',1,0,50),
 ('outing','외출','time',1,0,60),
 ('absence','결근','day',0,0,70),
 ('other','기타','time',0,0,80)`,
		`CREATE TABLE IF NOT EXISTS leave_requests (
  leave_req_id        TEXT PRIMARY KEY,
  org_id              TEXT NOT NULL,
  user_id             TEXT NOT NULL,
  doc_no              TEXT NOT NULL UNIQUE,
  type_key            TEXT NOT NULL,
  type_note           TEXT,
  period_mode         TEXT NOT NULL,
  start_date          TEXT NOT NULL,
  end_date            TEXT NOT NULL,
  start_time          TEXT,
  end_time            TEXT,
  half_part           TEXT,
  work_minutes        INTEGER NOT NULL DEFAULT 0,
  swap_date           TEXT,
  swap_state          TEXT,
  reason              TEXT NOT NULL,
  destination         TEXT,
  contact_phone       TEXT,
  deputy_user_id      TEXT,
  approver_deputy_user_id TEXT,
  approval_request_id TEXT,
  status              TEXT NOT NULL DEFAULT 'draft',
  created_at          DATETIME DEFAULT CURRENT_TIMESTAMP
)`,
		`CREATE INDEX IF NOT EXISTS idx_lreq_user ON leave_requests(user_id, start_date)`,
		`CREATE INDEX IF NOT EXISTS idx_lreq_org ON leave_requests(org_id, status)`,
		`CREATE TABLE IF NOT EXISTS leave_request_days (
  leave_req_id TEXT NOT NULL,
  leave_date   TEXT NOT NULL,
  minutes      INTEGER NOT NULL,
  PRIMARY KEY (leave_req_id, leave_date)
)`,
		`CREATE TABLE IF NOT EXISTS doc_sequences (
  scope TEXT NOT NULL, seq_date TEXT NOT NULL, last_no INTEGER NOT NULL,
  PRIMARY KEY (scope, seq_date)
)`,
		`CREATE TRIGGER IF NOT EXISTS trg_lreq_frozen BEFORE UPDATE ON leave_requests
WHEN OLD.status <> 'draft' AND (
  NEW.type_key IS NOT OLD.type_key OR NEW.start_date IS NOT OLD.start_date
  OR NEW.end_date IS NOT OLD.end_date OR NEW.reason IS NOT OLD.reason
  OR NEW.period_mode IS NOT OLD.period_mode
)
BEGIN SELECT RAISE(ABORT, 'frozen'); END`,
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			log.Printf("059 leave: %v", err)
			return
		}
	}
	if _, err := db.Exec(`ALTER TABLE staff_leaves ADD COLUMN source_request_id TEXT`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			log.Printf("059 staff_leaves.source_request_id: %v", err)
		}
	}
}

func leaveMigrationFiles() (up, down string, err error) {
	candidates := []string{
		filepath.Join("..", "..", "migrations"),
		filepath.Join("migrations"),
	}
	var dir string
	for _, c := range candidates {
		if _, e := os.Stat(filepath.Join(c, "059_leave_requests.sql")); e == nil {
			dir = c
			break
		}
	}
	if dir == "" {
		return "", "", os.ErrNotExist
	}
	ub, err := os.ReadFile(filepath.Join(dir, "059_leave_requests.sql"))
	if err != nil {
		return "", "", err
	}
	dbt, err := os.ReadFile(filepath.Join(dir, "059_leave_requests.down.sql"))
	if err != nil {
		return "", "", err
	}
	return string(ub), string(dbt), nil
}
