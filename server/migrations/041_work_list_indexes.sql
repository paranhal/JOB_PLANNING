-- §38.9 오늘 내 업무 조회 인덱스.
-- 실제 앱은 InitDB 시 repository.applyWorkListIndexes 가 동일 내용을 실행한다.

CREATE INDEX IF NOT EXISTS idx_as_receipts_status_visit
	ON as_receipts(status, visit_scheduled_date);
CREATE INDEX IF NOT EXISTS idx_as_receipts_assigned_to
	ON as_receipts(assigned_to);
CREATE INDEX IF NOT EXISTS idx_as_receipts_assigned_user_id
	ON as_receipts(assigned_user_id);
CREATE INDEX IF NOT EXISTS idx_as_receipts_complete_datetime
	ON as_receipts(complete_datetime);
CREATE INDEX IF NOT EXISTS idx_maintenance_visits_completed_date
	ON maintenance_visits(completed, visit_date);
CREATE INDEX IF NOT EXISTS idx_maintenance_visits_assignee
	ON maintenance_visits(assignee);
CREATE INDEX IF NOT EXISTS idx_as_processes_as_id
	ON as_processes(as_id);
CREATE INDEX IF NOT EXISTS idx_as_work_items_status_as
	ON as_work_items(status, as_id);
