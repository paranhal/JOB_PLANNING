-- 95-45: 기간 주기 사이트의 월 점검 대상 등록.
CREATE TABLE IF NOT EXISTS maintenance_month_targets (
	target_id TEXT PRIMARY KEY,
	plan_id TEXT NOT NULL,
	org_id TEXT NOT NULL DEFAULT '',
	customer_id TEXT NOT NULL,
	slot_key TEXT NOT NULL DEFAULT 'generic',
	year INTEGER NOT NULL,
	month INTEGER NOT NULL,
	round_no INTEGER NOT NULL DEFAULT 1,
	source TEXT NOT NULL DEFAULT 'site',
	region TEXT NOT NULL DEFAULT '',
	created_by_user_id TEXT NOT NULL DEFAULT '',
	created_by_name TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
	cancelled_at TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_mnt_month_targets_active
	ON maintenance_month_targets(plan_id, customer_id, slot_key, year, month, round_no)
	WHERE cancelled_at IS NULL OR cancelled_at = '';
CREATE INDEX IF NOT EXISTS idx_mnt_month_targets_plan_ym
	ON maintenance_month_targets(plan_id, year, month);
