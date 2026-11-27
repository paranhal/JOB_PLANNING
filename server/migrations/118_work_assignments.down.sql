-- 95-225 되돌리기: 배정 테이블 제거(공동 작업 컬럼 collab_scope 는 SQLite 제약상 남긴다)
DROP INDEX IF EXISTS ux_wa_active_target;
DROP INDEX IF EXISTS idx_wa_task;
DROP INDEX IF EXISTS idx_wa_to_status;
DROP TABLE IF EXISTS work_assignments;
DROP TABLE IF EXISTS work_collab_members;
