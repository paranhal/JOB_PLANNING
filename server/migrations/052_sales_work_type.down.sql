DROP INDEX IF EXISTS idx_work_tasks_source_role;
CREATE UNIQUE INDEX IF NOT EXISTS idx_work_tasks_source ON work_tasks(source_type, source_id)
	WHERE source_type IS NOT NULL AND source_type != '';

UPDATE work_tasks SET work_type = 'admin', source_role = ''
 WHERE source_type = 'sales_activity' AND work_type = 'sales';

DELETE FROM schema_migrations WHERE version = 52;
