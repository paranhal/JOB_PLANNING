DROP INDEX IF EXISTS idx_as_kb_gaps_resolved;
DROP INDEX IF EXISTS idx_as_kb_gaps_open;
DROP TABLE IF EXISTS as_kb_gaps;
DELETE FROM schema_migrations WHERE version=45;
