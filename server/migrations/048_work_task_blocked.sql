-- 048 막힌 이유. 밀린 것과 기다리는 것을 가른다. §42.7

ALTER TABLE work_tasks ADD COLUMN blocked_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE work_tasks ADD COLUMN blocked_at TEXT NOT NULL DEFAULT '';

INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (48, datetime('now'));
