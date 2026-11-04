-- 95-119 업무 사이트 완료 체크 (여러 날 × 사이트)
CREATE TABLE IF NOT EXISTS work_task_site_progress (
  task_id     TEXT NOT NULL,
  customer_id TEXT NOT NULL,
  work_date   TEXT NOT NULL,
  status      TEXT NOT NULL DEFAULT 'pending',
  checked_by  TEXT NOT NULL DEFAULT '',
  checked_at  TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (task_id, customer_id, work_date)
);
CREATE INDEX IF NOT EXISTS ix_wtsp_task ON work_task_site_progress(task_id);
CREATE INDEX IF NOT EXISTS ix_wtsp_date ON work_task_site_progress(task_id, work_date);

CREATE TABLE IF NOT EXISTS work_task_progress_flag (
  task_id TEXT PRIMARY KEY,
  track   INTEGER NOT NULL DEFAULT 1
);
