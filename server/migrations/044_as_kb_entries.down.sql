DROP INDEX IF EXISTS idx_as_kb_as;
DROP INDEX IF EXISTS idx_as_kb_current;
DROP TABLE IF EXISTS as_kb_entries;
DELETE FROM schema_migrations WHERE version=44;
