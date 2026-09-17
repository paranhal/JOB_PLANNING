-- 050 담당자 이름 → user_id. §44.8.3 1·2걸음.
-- 실제 앱은 InitDB 시 repository.applyAssigneeUserIDs 가 동일 내용을 실행한다.
-- 3걸음(이름 열 삭제·work_task_members 재생성)은 매칭 실패 0건·건수 일치 후에만 코드로 실행한다.

ALTER TABLE work_task_members ADD COLUMN user_id TEXT;
ALTER TABLE work_tasks ADD COLUMN assignee_user_id TEXT;
ALTER TABLE maintenance_visits ADD COLUMN assignee_user_id TEXT;
ALTER TABLE work_actions ADD COLUMN assignee_user_id TEXT;
ALTER TABLE as_processes ADD COLUMN worker_user_id TEXT;
ALTER TABLE work_activities ADD COLUMN actor_user_id TEXT;
ALTER TABLE as_receipts ADD COLUMN external_assignee TEXT;

INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (50, datetime('now'));
