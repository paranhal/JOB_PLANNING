-- 049 화면 조회 인덱스. §44.7
-- 실제 앱은 InitDB 시 repository.applyQueryPlanIndexes 가 동일 내용을 실행한다.

CREATE INDEX IF NOT EXISTS idx_as_processes_as_id ON as_processes(as_id);
CREATE INDEX IF NOT EXISTS idx_as_receipts_customer_id ON as_receipts(customer_id);
CREATE INDEX IF NOT EXISTS idx_assets_customer_id ON assets(customer_id);
CREATE INDEX IF NOT EXISTS idx_customer_rooms_floor_id ON customer_rooms(floor_id);
CREATE INDEX IF NOT EXISTS idx_customer_floors_bld_id ON customer_floors(building_id);
CREATE INDEX IF NOT EXISTS idx_customer_buildings_cust ON customer_buildings(customer_id);
CREATE INDEX IF NOT EXISTS idx_attachments_ref ON attachments(ref_type, ref_id);
CREATE INDEX IF NOT EXISTS idx_as_receipts_asset_id ON as_receipts(asset_id);
CREATE INDEX IF NOT EXISTS idx_as_receipts_receipt_dt ON as_receipts(receipt_datetime);
CREATE INDEX IF NOT EXISTS idx_contacts_customer_id ON contacts(customer_id);
CREATE INDEX IF NOT EXISTS idx_contact_history_contact ON contact_history(contact_id);
CREATE INDEX IF NOT EXISTS idx_maint_visits_customer ON maintenance_visits(customer_id);
CREATE INDEX IF NOT EXISTS idx_as_work_items_as_id ON as_work_items(as_id);
CREATE INDEX IF NOT EXISTS idx_customers_parent ON customers(parent_customer_id);
CREATE INDEX IF NOT EXISTS idx_work_tasks_project_id ON work_tasks(project_id);

CREATE INDEX IF NOT EXISTS idx_as_receipts_open
  ON as_receipts(status, receipt_datetime)
  WHERE status NOT IN ('completed','closed','cancelled');

DROP INDEX IF EXISTS idx_maintenance_visit_dedup2;
DROP INDEX IF EXISTS idx_data_backups_folder;

ANALYZE;

INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (49, datetime('now'));
