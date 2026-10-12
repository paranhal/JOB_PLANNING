package repository

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func applyApprovalEngineV71(db *sql.DB) {
	if db == nil {
		return
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS employee_profiles (
  user_id      TEXT PRIMARY KEY,
  org_id       TEXT NOT NULL,
  hire_date    TEXT,
  is_org_head  INTEGER NOT NULL DEFAULT 0,
  phone        TEXT,
  updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP
)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_employee_one_head
  ON employee_profiles(org_id) WHERE is_org_head=1`,
		`CREATE TABLE IF NOT EXISTS approval_policies (
  org_id                     TEXT PRIMARY KEY,
  deleg_by_head              INTEGER NOT NULL DEFAULT 1,
  deleg_by_admin             INTEGER NOT NULL DEFAULT 1,
  deleg_with_leave           INTEGER NOT NULL DEFAULT 0,
  admin_emergency_approve    INTEGER NOT NULL DEFAULT 0,
  self_approve_leave         INTEGER NOT NULL DEFAULT 1,
  self_approve_quote         INTEGER NOT NULL DEFAULT 1,
  notify_admin_self_approve  INTEGER NOT NULL DEFAULT 0,
  updated_by                 TEXT,
  updated_at                 DATETIME DEFAULT CURRENT_TIMESTAMP
)`,
		`CREATE TABLE IF NOT EXISTS approval_templates (
  template_id   TEXT PRIMARY KEY,
  org_id        TEXT NOT NULL,
  doc_type      TEXT NOT NULL,
  name          TEXT NOT NULL,
  is_active     INTEGER NOT NULL DEFAULT 1,
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
)`,
		`CREATE TABLE IF NOT EXISTS approval_template_steps (
  template_id   TEXT NOT NULL,
  step_no       INTEGER NOT NULL,
  step_type     TEXT NOT NULL DEFAULT 'approve',
  approver_rule TEXT NOT NULL,
  PRIMARY KEY (template_id, step_no)
)`,
		`CREATE TABLE IF NOT EXISTS approval_requests (
  request_id    TEXT PRIMARY KEY,
  org_id        TEXT NOT NULL,
  doc_type      TEXT NOT NULL,
  doc_id        TEXT NOT NULL,
  revision      INTEGER NOT NULL DEFAULT 1,
  drafter_id    TEXT NOT NULL,
  title         TEXT NOT NULL,
  status        TEXT NOT NULL DEFAULT 'draft',
  snapshot_json TEXT,
  submitted_at  DATETIME,
  completed_at  DATETIME,
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (doc_type, doc_id, revision)
)`,
		`CREATE INDEX IF NOT EXISTS idx_apr_status ON approval_requests(org_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_apr_doc ON approval_requests(doc_type, doc_id)`,
		`CREATE TABLE IF NOT EXISTS approval_steps (
  step_id       TEXT PRIMARY KEY,
  request_id    TEXT NOT NULL REFERENCES approval_requests(request_id),
  step_no       INTEGER NOT NULL,
  step_type     TEXT NOT NULL DEFAULT 'approve',
  approver_id   TEXT NOT NULL,
  acted_by      TEXT,
  act_mode      TEXT,
  status        TEXT NOT NULL DEFAULT 'waiting',
  comment       TEXT,
  signature_ref TEXT,
  acted_at      DATETIME,
  UNIQUE (request_id, step_no)
)`,
		`CREATE INDEX IF NOT EXISTS idx_aps_approver ON approval_steps(approver_id, status)`,
		`CREATE TABLE IF NOT EXISTS approval_delegations (
  delegation_id TEXT PRIMARY KEY,
  org_id        TEXT NOT NULL,
  from_user_id  TEXT NOT NULL,
  to_user_id    TEXT NOT NULL,
  start_date    TEXT NOT NULL,
  end_date      TEXT NOT NULL,
  source        TEXT NOT NULL,
  leave_req_id  TEXT,
  created_by    TEXT NOT NULL,
  revoked_at    DATETIME,
  revoked_by    TEXT,
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
)`,
		`CREATE INDEX IF NOT EXISTS idx_deleg_from ON approval_delegations(from_user_id, start_date, end_date)`,
		`CREATE TABLE IF NOT EXISTS approval_events (
  event_id      TEXT PRIMARY KEY,
  request_id    TEXT NOT NULL,
  event_type    TEXT NOT NULL,
  actor_id      TEXT NOT NULL,
  detail_json   TEXT,
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
)`,
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			log.Printf("058 approval: %v", err)
			return
		}
	}
	applyApprovalTriggers(db)
	if _, err := db.Exec(`INSERT OR IGNORE INTO role_permissions(role, perm_key, access, updated_by)
		VALUES ('org_admin', 'approval.admin', 3, 'system-058')`); err != nil {
		log.Printf("058 approval.admin seed: %v", err)
	}
}

func applyApprovalTriggers(db *sql.DB) {
	trigs := []string{
		`CREATE TRIGGER IF NOT EXISTS trg_apr_events_no_update BEFORE UPDATE ON approval_events
BEGIN SELECT RAISE(ABORT, 'append-only'); END`,
		`CREATE TRIGGER IF NOT EXISTS trg_apr_events_no_delete BEFORE DELETE ON approval_events
BEGIN SELECT RAISE(ABORT, 'append-only'); END`,
		`CREATE TRIGGER IF NOT EXISTS trg_apr_snap_lock BEFORE UPDATE OF snapshot_json ON approval_requests
WHEN OLD.status <> 'draft' AND IFNULL(NEW.snapshot_json,'') IS NOT IFNULL(OLD.snapshot_json,'')
BEGIN SELECT RAISE(ABORT, 'snapshot locked'); END`,
		`CREATE TRIGGER IF NOT EXISTS trg_apr_one_pending_ins BEFORE INSERT ON approval_steps
WHEN NEW.status = 'pending'
 AND EXISTS (SELECT 1 FROM approval_steps WHERE request_id=NEW.request_id AND status='pending')
BEGIN SELECT RAISE(ABORT, 'one pending'); END`,
		`CREATE TRIGGER IF NOT EXISTS trg_apr_one_pending_upd BEFORE UPDATE OF status ON approval_steps
WHEN NEW.status = 'pending' AND OLD.status <> 'pending'
 AND EXISTS (SELECT 1 FROM approval_steps WHERE request_id=NEW.request_id AND status='pending' AND step_id<>NEW.step_id)
BEGIN SELECT RAISE(ABORT, 'one pending'); END`,
		`CREATE TRIGGER IF NOT EXISTS trg_apr_in_progress_steps BEFORE UPDATE OF status ON approval_requests
WHEN NEW.status='in_progress'
 AND (SELECT COUNT(*) FROM approval_steps WHERE request_id=NEW.request_id AND step_type='approve')=0
BEGIN SELECT RAISE(ABORT, 'in_progress needs approve step'); END`,
		`CREATE TRIGGER IF NOT EXISTS trg_deleg_no_overlap_ins BEFORE INSERT ON approval_delegations
WHEN NEW.revoked_at IS NULL
 AND EXISTS (
   SELECT 1 FROM approval_delegations d
   WHERE d.from_user_id=NEW.from_user_id AND d.revoked_at IS NULL
     AND d.start_date <= NEW.end_date AND d.end_date >= NEW.start_date
 )
BEGIN SELECT RAISE(ABORT, 'delegation overlap'); END`,
		`CREATE TRIGGER IF NOT EXISTS trg_deleg_no_overlap_upd BEFORE UPDATE ON approval_delegations
WHEN NEW.revoked_at IS NULL
 AND EXISTS (
   SELECT 1 FROM approval_delegations d
   WHERE d.from_user_id=NEW.from_user_id AND d.revoked_at IS NULL
     AND d.delegation_id<>NEW.delegation_id
     AND d.start_date <= NEW.end_date AND d.end_date >= NEW.start_date
 )
BEGIN SELECT RAISE(ABORT, 'delegation overlap'); END`,
		`CREATE TRIGGER IF NOT EXISTS trg_head_role_ins BEFORE INSERT ON employee_profiles
WHEN NEW.is_org_head=1
 AND NOT EXISTS (
   SELECT 1 FROM users u WHERE u.user_id=NEW.user_id
     AND u.role IN ('org_admin','admin')
     AND COALESCE(u.is_readonly,0)=0
 )
BEGIN SELECT RAISE(ABORT, 'org head must be writable org_admin'); END`,
		`CREATE TRIGGER IF NOT EXISTS trg_head_role_upd BEFORE UPDATE ON employee_profiles
WHEN NEW.is_org_head=1
 AND NOT EXISTS (
   SELECT 1 FROM users u WHERE u.user_id=NEW.user_id
     AND u.role IN ('org_admin','admin')
     AND COALESCE(u.is_readonly,0)=0
 )
BEGIN SELECT RAISE(ABORT, 'org head must be writable org_admin'); END`,
	}
	for _, q := range trigs {
		if _, err := db.Exec(q); err != nil {
			log.Printf("058 trigger: %v", err)
		}
	}
}

func approvalMigrationFiles() (up, down string, err error) {
	candidates := []string{
		filepath.Join("..", "..", "migrations"),
		filepath.Join("migrations"),
	}
	var dir string
	for _, c := range candidates {
		if _, e := os.Stat(filepath.Join(c, "058_approval_engine.sql")); e == nil {
			dir = c
			break
		}
	}
	if dir == "" {
		return "", "", os.ErrNotExist
	}
	ub, err := os.ReadFile(filepath.Join(dir, "058_approval_engine.sql"))
	if err != nil {
		return "", "", err
	}
	dbt, err := os.ReadFile(filepath.Join(dir, "058_approval_engine.down.sql"))
	if err != nil {
		return "", "", err
	}
	return string(ub), string(dbt), nil
}

func execSQLScript(db *sql.DB, script string) error {
	for _, part := range strings.Split(script, ";") {
		q := strings.TrimSpace(part)
		if q == "" || strings.HasPrefix(q, "--") {
			continue
		}
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}
