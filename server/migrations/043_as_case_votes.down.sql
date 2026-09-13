DROP INDEX IF EXISTS idx_as_case_votes_as;
DROP TABLE IF EXISTS as_case_votes;
DELETE FROM schema_migrations WHERE version=43;
