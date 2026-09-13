-- 043 도움이 된 사례 추천. §41.4
CREATE TABLE IF NOT EXISTS as_case_votes (
	as_id      TEXT NOT NULL,
	user_id    TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (as_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_as_case_votes_as ON as_case_votes(as_id);

INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (43, datetime('now'));
