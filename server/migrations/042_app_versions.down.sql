-- 042 롤백. 배포 이력 표를 제거하고 스키마 번호 42 를 뺀다.

DROP INDEX IF EXISTS idx_app_versions_build;
DROP TABLE IF EXISTS app_versions;
DELETE FROM schema_migrations WHERE version=42;
