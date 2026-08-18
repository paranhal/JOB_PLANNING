package auditlog

// access.db 스키마. 업무 DB(app.db)와 파일·테이블을 분리한다(§25.3.1).
var schemaStmts = []string{
	`CREATE TABLE IF NOT EXISTS access_logs (
    seq            INTEGER PRIMARY KEY AUTOINCREMENT,
    log_id         TEXT    NOT NULL UNIQUE,
    user_id        TEXT    NOT NULL DEFAULT '',
    username       TEXT    NOT NULL DEFAULT '',
    full_name      TEXT    NOT NULL DEFAULT '',
    role           TEXT    NOT NULL DEFAULT '',
    access_at      TEXT    NOT NULL,
    client_ip      TEXT    NOT NULL DEFAULT '',
    forwarded_for  TEXT    NOT NULL DEFAULT '',
    user_agent     TEXT    NOT NULL DEFAULT '',
    session_id     TEXT    NOT NULL DEFAULT '',
    subject_type   TEXT    NOT NULL DEFAULT '',
    subject_id     TEXT    NOT NULL DEFAULT '',
    subject_name   TEXT    NOT NULL DEFAULT '',
    action         TEXT    NOT NULL,
    target_table   TEXT    NOT NULL DEFAULT '',
    target_id      TEXT    NOT NULL DEFAULT '',
    detail         TEXT    NOT NULL DEFAULT '',
    before_json    TEXT    NOT NULL DEFAULT '',
    after_json     TEXT    NOT NULL DEFAULT '',
    reason         TEXT    NOT NULL DEFAULT '',
    result         TEXT    NOT NULL DEFAULT '성공',
    prev_hash      TEXT    NOT NULL DEFAULT '',
    row_hash       TEXT    NOT NULL
)`,
	`CREATE INDEX IF NOT EXISTS idx_access_logs_access_at ON access_logs(access_at)`,
	`CREATE INDEX IF NOT EXISTS idx_access_logs_action ON access_logs(action)`,
	`CREATE INDEX IF NOT EXISTS idx_access_logs_user ON access_logs(username)`,
}

const insertSQL = `INSERT INTO access_logs (
    log_id, user_id, username, full_name, role, access_at,
    client_ip, forwarded_for, user_agent, session_id,
    subject_type, subject_id, subject_name,
    action, target_table, target_id, detail,
    before_json, after_json, reason, result,
    prev_hash, row_hash
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
