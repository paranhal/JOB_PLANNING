-- 017 기간 내 반복 실행 (§13.15.1~§13.15.5)
-- 실행 작업은 새 표가 아니라 work_tasks 일자 하위업무다. 실제 적용은 InitDB → applyWorkRecurrence.

ALTER TABLE work_tasks ADD COLUMN recurrence_role TEXT;
ALTER TABLE work_tasks ADD COLUMN occurrence_seq INTEGER;
ALTER TABLE work_tasks ADD COLUMN occurrence_status TEXT;
ALTER TABLE work_tasks ADD COLUMN not_done_reason TEXT;

CREATE TABLE IF NOT EXISTS work_recurrence (
  task_id                  TEXT PRIMARY KEY,
  start_date               TEXT NOT NULL DEFAULT '',
  end_date                 TEXT NOT NULL DEFAULT '',
  rule_type                TEXT NOT NULL DEFAULT 'none',
  interval_n               INTEGER NOT NULL DEFAULT 1,
  weekdays                 TEXT NOT NULL DEFAULT '',
  holiday_policy           TEXT NOT NULL DEFAULT 'as_is',
  complete_policy          TEXT NOT NULL DEFAULT 'manual',
  progress_include_future  INTEGER NOT NULL DEFAULT 0,
  final_result             TEXT NOT NULL DEFAULT '',
  created_at               DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at               DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_work_tasks_recurrence_parent
  ON work_tasks(parent_task_id, recurrence_role);
