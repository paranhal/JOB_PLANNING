-- 95-230 되돌리기: 이 마이그레이션이 채운 접수·방문만 이전 값으로 복원(그 사이 사용자가 바꾼 건은 건드리지 않는다)
UPDATE as_receipts SET project_id=NULLIF((SELECT b.before_project FROM source_project_backfill_120 b WHERE b.source_type='as' AND b.source_id=as_receipts.as_id),'')
 WHERE as_id IN (SELECT source_id FROM source_project_backfill_120 b WHERE b.source_type='as' AND b.after_project=as_receipts.project_id);
UPDATE maintenance_visits SET project_id=NULLIF((SELECT b.before_project FROM source_project_backfill_120 b WHERE b.source_type='maintenance' AND b.source_id=maintenance_visits.visit_id),'')
 WHERE visit_id IN (SELECT source_id FROM source_project_backfill_120 b WHERE b.source_type='maintenance' AND b.after_project=maintenance_visits.project_id);
DROP TABLE IF EXISTS source_project_backfill_120;
DELETE FROM app_settings WHERE setting_key='migration_120_done';
