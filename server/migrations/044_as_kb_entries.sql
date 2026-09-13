-- 044 AS 지식 층. 원본 as_processes 는 고치지 않는다. §41.7~§41.11
CREATE TABLE IF NOT EXISTS as_kb_entries (
	kb_id          TEXT PRIMARY KEY,
	as_id          TEXT,
	symptom_text   TEXT NOT NULL DEFAULT '',
	action_text    TEXT NOT NULL DEFAULT '',
	origin         TEXT NOT NULL DEFAULT 'added',
	rev            INTEGER NOT NULL DEFAULT 1,
	is_current     INTEGER NOT NULL DEFAULT 1,
	prev_kb_id     TEXT,
	author_id      TEXT NOT NULL DEFAULT '',
	author_name    TEXT NOT NULL DEFAULT '',
	created_at     TEXT NOT NULL DEFAULT '',
	change_note    TEXT NOT NULL DEFAULT '',
	status         TEXT NOT NULL DEFAULT 'published',
	helpful_count  INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_as_kb_current ON as_kb_entries(is_current, status);
CREATE INDEX IF NOT EXISTS idx_as_kb_as ON as_kb_entries(as_id);

INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (44, datetime('now'));
