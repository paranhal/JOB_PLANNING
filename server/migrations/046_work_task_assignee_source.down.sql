-- 046 롤백. SQLite는 DROP COLUMN을 버전마다 지원하지 않으므로 값만 비우고 번호를 되돌린다.
-- 컬럼 자체는 InitDB ALTER 가 다시 넣을 수 있다.

UPDATE work_tasks SET assignee_source = '' WHERE TRIM(COALESCE(assignee_source,'')) != '';
DELETE FROM schema_migrations WHERE version = 46;
