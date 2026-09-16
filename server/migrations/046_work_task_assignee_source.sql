-- 046 AS 연결 일일업무 담당자 출처. §42.3
-- as: AS 접수에서 따라옴. manual: 일일 업무에서 직접 바꿈(AS 변경에 덮이지 않음).

ALTER TABLE work_tasks ADD COLUMN assignee_source TEXT NOT NULL DEFAULT '';

UPDATE work_tasks
   SET assignee_source = 'as'
 WHERE source_type = 'as'
   AND TRIM(COALESCE(assignee_source,'')) = '';

INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (46, datetime('now'));
