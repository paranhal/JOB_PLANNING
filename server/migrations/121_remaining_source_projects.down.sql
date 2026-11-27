-- 95-231 되돌리기: 이 마이그레이션이 채운 것만 이전 값으로 복원(그 사이 사용자가 바꾼 건은 건드리지 않는다)
UPDATE work_tasks SET activity_kind=NULLIF((SELECT b.before_value FROM source_remaining_backfill_121 b WHERE b.kind='task_activity' AND b.ref_id=work_tasks.task_id),'')
 WHERE task_id IN (SELECT ref_id FROM source_remaining_backfill_121 b WHERE b.kind='task_activity' AND b.after_value=work_tasks.activity_kind);
UPDATE work_tasks SET project_id=NULLIF((SELECT b.before_value FROM source_remaining_backfill_121 b WHERE b.kind='task_project' AND b.ref_id=work_tasks.task_id),'')
 WHERE task_id IN (SELECT ref_id FROM source_remaining_backfill_121 b WHERE b.kind='task_project' AND b.after_value=work_tasks.project_id);
UPDATE as_receipts SET project_id=NULLIF((SELECT b.before_value FROM source_remaining_backfill_121 b WHERE b.kind='receipt_project' AND b.ref_id=as_receipts.as_id),'')
 WHERE as_id IN (SELECT ref_id FROM source_remaining_backfill_121 b WHERE b.kind='receipt_project' AND b.after_value=as_receipts.project_id);
DROP TABLE IF EXISTS source_remaining_backfill_121;
DELETE FROM app_settings WHERE setting_key='migration_121_done';
