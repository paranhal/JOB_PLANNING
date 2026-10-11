-- 057 계정 수명주기: 등록 요청 · 소프트 삭제 · 아이디 영구 예약 (개정안 §74)
ALTER TABLE users ADD COLUMN deleted_at DATETIME;
ALTER TABLE users ADD COLUMN deleted_by TEXT;
ALTER TABLE users ADD COLUMN delete_reason TEXT;
ALTER TABLE users ADD COLUMN delete_snapshot TEXT;
ALTER TABLE users ADD COLUMN must_change_password INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS account_requests (
  request_id      TEXT PRIMARY KEY,
  org_id          TEXT NOT NULL,
  requested_by    TEXT NOT NULL,
  full_name       TEXT NOT NULL,
  username        TEXT NOT NULL,
  role            TEXT NOT NULL,
  mobile          TEXT,
  reason          TEXT,
  status          TEXT NOT NULL DEFAULT 'pending',
  restore_user_id TEXT,
  decided_by      TEXT,
  decided_at      DATETIME,
  decision_note   TEXT,
  created_user_id TEXT,
  created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_account_requests_status ON account_requests(status, created_at);

CREATE TABLE IF NOT EXISTS user_lifecycle_events (
  event_id   TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL,
  kind       TEXT NOT NULL,
  actor_id   TEXT,
  note       TEXT,
  detail     TEXT,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_user_lifecycle_user ON user_lifecycle_events(user_id, created_at);

CREATE TABLE IF NOT EXISTS username_reservations (
  username_norm TEXT PRIMARY KEY,
  user_id       TEXT NOT NULL,
  reason        TEXT NOT NULL,
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);
