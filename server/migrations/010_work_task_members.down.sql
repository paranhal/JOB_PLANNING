-- 010 롤백. work_tasks.assignee 는 그대로 둔다.

DROP TRIGGER IF EXISTS trg_wt_insert_owner_member;
DROP TRIGGER IF EXISTS trg_wt_update_owner_assignee;
DROP TRIGGER IF EXISTS trg_wt_delete_members;
DROP TRIGGER IF EXISTS trg_wtm_reject_second_owner;
DROP TRIGGER IF EXISTS trg_wtm_require_owner_on_support;
DROP TRIGGER IF EXISTS trg_wtm_reject_owner_role_upd;
DROP TRIGGER IF EXISTS trg_wtm_reject_demote_last_owner;
DROP TRIGGER IF EXISTS trg_wtm_reject_delete_last_owner;
DROP INDEX IF EXISTS idx_work_task_members_assignee;
DROP INDEX IF EXISTS idx_work_task_members_role;
DROP TABLE IF EXISTS work_task_members;
