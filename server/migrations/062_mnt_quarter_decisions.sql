-- 76-B D-71. 분기 점검 「이번 분기 건너뜀」 기록.

CREATE TABLE IF NOT EXISTS maintenance_quarter_decisions (
	decision_id TEXT PRIMARY KEY,
	plan_id TEXT NOT NULL,
	customer_id TEXT NOT NULL,
	slot_key TEXT NOT NULL,
	year INTEGER NOT NULL,
	quarter INTEGER NOT NULL,
	action TEXT NOT NULL DEFAULT 'skip',
	reason TEXT NOT NULL DEFAULT '',
	created_by TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_mnt_quarter_decisions
	ON maintenance_quarter_decisions(plan_id, customer_id, slot_key, year, quarter);
