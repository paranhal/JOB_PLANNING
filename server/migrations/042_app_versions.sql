-- 042 배포 이력 · 스키마 번호. §40.5
-- 실제 적용은 InitDB → applyAppVersions.

CREATE TABLE IF NOT EXISTS schema_migrations (
	version    INTEGER PRIMARY KEY,
	applied_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS app_versions (
	version_id       TEXT PRIMARY KEY,
	version          TEXT NOT NULL DEFAULT '',
	"commit"         TEXT NOT NULL DEFAULT '',
	built_at         TEXT NOT NULL DEFAULT '',
	first_started_at TEXT NOT NULL DEFAULT '',
	last_started_at  TEXT NOT NULL DEFAULT '',
	start_count      INTEGER NOT NULL DEFAULT 1,
	schema_version   INTEGER NOT NULL DEFAULT 0,
	host             TEXT NOT NULL DEFAULT '',
	note             TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_app_versions_build
	ON app_versions(version, "commit", built_at);

INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (42, datetime('now'));
