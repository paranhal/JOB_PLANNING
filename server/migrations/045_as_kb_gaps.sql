-- 045 답 없는 검색어. 결과 0건이면 자동 기록. §41.13
CREATE TABLE IF NOT EXISTS as_kb_gaps (
	gap_id          TEXT PRIMARY KEY,
	query           TEXT NOT NULL DEFAULT '',
	query_norm      TEXT NOT NULL DEFAULT '',
	searched_by     TEXT NOT NULL DEFAULT '',
	searched_at     TEXT NOT NULL DEFAULT '',
	hit_count       INTEGER NOT NULL DEFAULT 0,
	as_id           TEXT,
	search_count    INTEGER NOT NULL DEFAULT 1,
	resolved_kb_id  TEXT,
	resolved_at     TEXT
);

CREATE INDEX IF NOT EXISTS idx_as_kb_gaps_open ON as_kb_gaps(query_norm, hit_count);
CREATE INDEX IF NOT EXISTS idx_as_kb_gaps_resolved ON as_kb_gaps(resolved_kb_id);

INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (45, datetime('now'));
