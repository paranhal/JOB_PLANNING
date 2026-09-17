-- 047 배정 알림. 화면 안에서만. 메일·푸시 없음. §42.5 · §13.15.12

CREATE TABLE IF NOT EXISTS work_assign_notices (
	notice_id    TEXT PRIMARY KEY,
	user_id      TEXT NOT NULL,
	source_type  TEXT NOT NULL,
	source_id    TEXT NOT NULL,
	assigned_by  TEXT NOT NULL DEFAULT '',
	assigned_at  TEXT NOT NULL,
	seen_at      TEXT,
	acted_at     TEXT
);

CREATE INDEX IF NOT EXISTS idx_work_assign_notices_unseen
	ON work_assign_notices(user_id, seen_at);

CREATE INDEX IF NOT EXISTS idx_work_assign_notices_source
	ON work_assign_notices(source_type, source_id);

INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (47, datetime('now'));
