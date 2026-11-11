-- 95-175 B / 95-121. 일 실적 이벤트. 사이트 완료의 원본 체크는 work_task_site_progress 다.
-- 작업 항목 1줄=사이트 1곳 방식은 쓰지 않는다.
CREATE TABLE IF NOT EXISTS package_events (
  event_id TEXT PRIMARY KEY,
  package_id TEXT NOT NULL,
  package_site_id TEXT NOT NULL DEFAULT '',
  plan_id TEXT NOT NULL DEFAULT '',
  event_date TEXT NOT NULL,
  event_type TEXT NOT NULL,
  qty_delta INTEGER NOT NULL DEFAULT 0,
  user_id TEXT NOT NULL DEFAULT '',
  work_task_id TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT 'work',
  note TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pkg_events_pkg_date ON package_events(package_id, event_date);
CREATE INDEX IF NOT EXISTS idx_pkg_events_site ON package_events(package_site_id);
CREATE INDEX IF NOT EXISTS idx_pkg_events_plan ON package_events(plan_id, event_date);

CREATE TABLE IF NOT EXISTS package_progress_daily (
  package_id TEXT NOT NULL,
  plan_id TEXT NOT NULL DEFAULT '',
  snap_date TEXT NOT NULL,
  total_sites INTEGER NOT NULL DEFAULT 0,
  done_sites INTEGER NOT NULL DEFAULT 0,
  doing_sites INTEGER NOT NULL DEFAULT 0,
  total_qty INTEGER NOT NULL DEFAULT 0,
  done_qty INTEGER NOT NULL DEFAULT 0,
  day_done_sites INTEGER NOT NULL DEFAULT 0,
  day_done_qty INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL,
  PRIMARY KEY(package_id, plan_id, snap_date)
);
