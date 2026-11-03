-- 95-78 업무 끼워넣기·구간 분리·작업 리스트. 시드 INSERT 없음. 운영 행은 보정하지 않는다.

CREATE TABLE IF NOT EXISTS work_task_segments (
  segment_id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  kind TEXT NOT NULL DEFAULT 'plan',
  scope TEXT NOT NULL DEFAULT 'common',
  seq INTEGER NOT NULL DEFAULT 1,
  work_date TEXT NOT NULL DEFAULT '',
  start_time TEXT NOT NULL DEFAULT '',
  end_time TEXT NOT NULL DEFAULT '',
  duration_min INTEGER NOT NULL DEFAULT 0,
  travel_before_min INTEGER NOT NULL DEFAULT 0,
  travel_after_min INTEGER NOT NULL DEFAULT 0,
  user_id TEXT NOT NULL DEFAULT '',
  assignee TEXT NOT NULL DEFAULT '',
  worker_user_id TEXT NOT NULL DEFAULT '',
  worker_name TEXT NOT NULL DEFAULT '',
  interrupt_task_id TEXT NOT NULL DEFAULT '',
  note TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
  FOREIGN KEY (task_id) REFERENCES work_tasks(task_id)
);
CREATE INDEX IF NOT EXISTS idx_wts_task ON work_task_segments(task_id, kind, seq);
CREATE INDEX IF NOT EXISTS idx_wts_interrupt ON work_task_segments(interrupt_task_id);

CREATE TABLE IF NOT EXISTS work_task_departures (
  departure_id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  user_id TEXT NOT NULL DEFAULT '',
  assignee TEXT NOT NULL DEFAULT '',
  work_date TEXT NOT NULL DEFAULT '',
  start_time TEXT NOT NULL DEFAULT '',
  end_time TEXT NOT NULL DEFAULT '',
  interrupt_task_id TEXT NOT NULL DEFAULT '',
  return_status TEXT NOT NULL DEFAULT 'pending',
  created_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
  FOREIGN KEY (task_id) REFERENCES work_tasks(task_id)
);
CREATE INDEX IF NOT EXISTS idx_wtd_task ON work_task_departures(task_id);
CREATE INDEX IF NOT EXISTS idx_wtd_return ON work_task_departures(return_status);

CREATE TABLE IF NOT EXISTS work_task_items (
  item_id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  seq INTEGER NOT NULL DEFAULT 1,
  title TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'todo',
  note TEXT NOT NULL DEFAULT '',
  copied_from_item_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
  FOREIGN KEY (task_id) REFERENCES work_tasks(task_id)
);
CREATE INDEX IF NOT EXISTS idx_wti_task ON work_task_items(task_id, seq);

CREATE TABLE IF NOT EXISTS work_task_item_segments (
  item_id TEXT NOT NULL,
  segment_id TEXT NOT NULL,
  PRIMARY KEY (item_id, segment_id)
);
