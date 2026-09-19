-- 052 영업활동 업무 구분 · 한 활동이 업무 둘(done/next)
ALTER TABLE work_tasks ADD COLUMN source_role TEXT NOT NULL DEFAULT '';

DROP INDEX IF EXISTS idx_work_tasks_source;
CREATE UNIQUE INDEX IF NOT EXISTS idx_work_tasks_source_role
	ON work_tasks(source_type, source_id, source_role)
	WHERE source_type IS NOT NULL AND source_type != '';

UPDATE work_tasks
   SET work_type = 'sales'
 WHERE source_type = 'sales_activity'
   AND work_type = 'admin';

UPDATE work_tasks
   SET source_role = 'done'
 WHERE source_type = 'sales_activity'
   AND TRIM(COALESCE(source_role,'')) = '';

UPDATE work_tasks
   SET status = 'complete',
       progress = 100,
       complete_date = work_date
 WHERE source_type = 'sales_activity'
   AND status = 'waiting'
   AND progress = 0
   AND IFNULL(work_date,'') <> ''
   AND work_date <= date('now','localtime');

INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (52, datetime('now'));
