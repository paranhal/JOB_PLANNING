-- 95-47: 정기점검 방문 시간·소요 시간, 점검 장비 다건.
ALTER TABLE maintenance_visits ADD COLUMN start_time TEXT NOT NULL DEFAULT '';
ALTER TABLE maintenance_visits ADD COLUMN end_time TEXT NOT NULL DEFAULT '';
ALTER TABLE maintenance_visits ADD COLUMN travel_min INTEGER NOT NULL DEFAULT 0;
ALTER TABLE maintenance_visits ADD COLUMN duration_min INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS maintenance_visit_targets (
	target_id TEXT PRIMARY KEY,
	visit_id TEXT NOT NULL,
	asset_id TEXT NOT NULL DEFAULT '',
	name TEXT NOT NULL DEFAULT '',
	memo TEXT NOT NULL DEFAULT '',
	sort_order INTEGER NOT NULL DEFAULT 0,
	org_id TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
);
CREATE INDEX IF NOT EXISTS idx_mnt_visit_targets_visit
	ON maintenance_visit_targets(visit_id, sort_order);
