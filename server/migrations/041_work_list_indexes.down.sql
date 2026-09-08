-- 041 롤백. 오늘 내 업무 조회 인덱스를 뗀다.

DROP INDEX IF EXISTS idx_as_receipts_status_visit;
DROP INDEX IF EXISTS idx_as_receipts_assigned_to;
DROP INDEX IF EXISTS idx_as_receipts_assigned_user_id;
DROP INDEX IF EXISTS idx_as_receipts_complete_datetime;
DROP INDEX IF EXISTS idx_maintenance_visits_completed_date;
DROP INDEX IF EXISTS idx_maintenance_visits_assignee;
DROP INDEX IF EXISTS idx_as_processes_as_id;
DROP INDEX IF EXISTS idx_as_work_items_status_as;
