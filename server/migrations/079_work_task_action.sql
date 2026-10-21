-- 95-11 작업 조치. 시드 INSERT 없음.

ALTER TABLE work_tasks ADD COLUMN work_place TEXT NOT NULL DEFAULT '';
ALTER TABLE work_tasks ADD COLUMN action_taken TEXT NOT NULL DEFAULT '';
ALTER TABLE work_tasks ADD COLUMN result_code TEXT NOT NULL DEFAULT '';
ALTER TABLE work_tasks ADD COLUMN complete_datetime TEXT NOT NULL DEFAULT '';
ALTER TABLE work_tasks ADD COLUMN customer_confirmer TEXT NOT NULL DEFAULT '';
ALTER TABLE work_tasks ADD COLUMN hold_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE work_tasks ADD COLUMN hold_next_action TEXT NOT NULL DEFAULT '';
ALTER TABLE work_tasks ADD COLUMN transfer_to TEXT NOT NULL DEFAULT '';
ALTER TABLE work_tasks ADD COLUMN cancel_reason TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS work_task_processes (
  process_id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  org_id TEXT NOT NULL DEFAULT '',
  process_at TEXT NOT NULL DEFAULT '',
  worker TEXT NOT NULL DEFAULT '',
  worker_user_id TEXT NOT NULL DEFAULT '',
  work_place TEXT NOT NULL DEFAULT '',
  result_code TEXT NOT NULL DEFAULT '',
  work_content TEXT NOT NULL DEFAULT '',
  note TEXT NOT NULL DEFAULT '',
  next_date TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS ix_wtp_task ON work_task_processes(task_id);
CREATE INDEX IF NOT EXISTS ix_wtp_org ON work_task_processes(org_id, task_id);

CREATE TABLE IF NOT EXISTS work_task_parts_used (
  part_id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  process_id TEXT NOT NULL DEFAULT '',
  item_id TEXT,
  part_name TEXT NOT NULL,
  part_code TEXT NOT NULL DEFAULT '',
  qty INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS ix_wtpu_task ON work_task_parts_used(task_id);
