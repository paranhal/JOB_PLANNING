-- 71-B 결재 엔진 · 직원 프로필(팀장). 056·057 은 AS 지원·계정 수명주기가 씀.

CREATE TABLE IF NOT EXISTS employee_profiles (
  user_id      TEXT PRIMARY KEY,
  org_id       TEXT NOT NULL,
  hire_date    TEXT,
  is_org_head  INTEGER NOT NULL DEFAULT 0,
  phone        TEXT,
  updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_employee_one_head
  ON employee_profiles(org_id) WHERE is_org_head=1;

CREATE TABLE IF NOT EXISTS approval_policies (
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
);

CREATE TABLE IF NOT EXISTS approval_templates (
  template_id   TEXT PRIMARY KEY,
  org_id        TEXT NOT NULL,
  doc_type      TEXT NOT NULL,
  name          TEXT NOT NULL,
  is_active     INTEGER NOT NULL DEFAULT 1,
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS approval_template_steps (
  template_id   TEXT NOT NULL,
  step_no       INTEGER NOT NULL,
  step_type     TEXT NOT NULL DEFAULT 'approve',
  approver_rule TEXT NOT NULL,
  PRIMARY KEY (template_id, step_no)
);

CREATE TABLE IF NOT EXISTS approval_requests (
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
);
CREATE INDEX IF NOT EXISTS idx_apr_status ON approval_requests(org_id, status);
CREATE INDEX IF NOT EXISTS idx_apr_doc ON approval_requests(doc_type, doc_id);

CREATE TABLE IF NOT EXISTS approval_steps (
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
);
CREATE INDEX IF NOT EXISTS idx_aps_approver ON approval_steps(approver_id, status);

CREATE TABLE IF NOT EXISTS approval_delegations (
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
);
CREATE INDEX IF NOT EXISTS idx_deleg_from ON approval_delegations(from_user_id, start_date, end_date);

CREATE TABLE IF NOT EXISTS approval_events (
  event_id      TEXT PRIMARY KEY,
  request_id    TEXT NOT NULL,
  event_type    TEXT NOT NULL,
  actor_id      TEXT NOT NULL,
  detail_json   TEXT,
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO role_permissions(role, perm_key, access, updated_by)
VALUES ('org_admin', 'approval.admin', 3, 'system-058');
