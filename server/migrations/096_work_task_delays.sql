-- 95-80 지연 사유·이월 이력. 시드 INSERT/운영 행 UPDATE 없음.

CREATE TABLE IF NOT EXISTS work_task_delays (
  delay_id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  org_id TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL DEFAULT 'reason',
  prev_work_date TEXT NOT NULL DEFAULT '',
  prev_due_date TEXT NOT NULL DEFAULT '',
  new_work_date TEXT NOT NULL DEFAULT '',
  new_due_date TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL DEFAULT '',
  overdue_days INTEGER NOT NULL DEFAULT 0,
  created_by_user_id TEXT NOT NULL DEFAULT '',
  created_by_name TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
);
CREATE INDEX IF NOT EXISTS idx_wtd_task ON work_task_delays(task_id, created_at);
CREATE INDEX IF NOT EXISTS idx_wtd_org ON work_task_delays(org_id, created_at);

ALTER TABLE work_tasks ADD COLUMN orig_work_date TEXT NOT NULL DEFAULT '';
ALTER TABLE work_tasks ADD COLUMN orig_due_date TEXT NOT NULL DEFAULT '';
ALTER TABLE work_tasks ADD COLUMN carry_count INTEGER NOT NULL DEFAULT 0;
