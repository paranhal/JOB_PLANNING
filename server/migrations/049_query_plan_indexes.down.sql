-- 049 롤백. 화면 조회 인덱스를 뗀다. UNIQUE 제약이 만드는 인덱스는 건드리지 않는다.

DROP INDEX IF EXISTS idx_as_receipts_open;
DROP INDEX IF EXISTS idx_work_tasks_project_id;
DROP INDEX IF EXISTS idx_customers_parent;
DROP INDEX IF EXISTS idx_as_work_items_as_id;
DROP INDEX IF EXISTS idx_maint_visits_customer;
DROP INDEX IF EXISTS idx_contact_history_contact;
DROP INDEX IF EXISTS idx_contacts_customer_id;
DROP INDEX IF EXISTS idx_as_receipts_receipt_dt;
DROP INDEX IF EXISTS idx_as_receipts_asset_id;
DROP INDEX IF EXISTS idx_attachments_ref;
DROP INDEX IF EXISTS idx_customer_buildings_cust;
DROP INDEX IF EXISTS idx_customer_floors_bld_id;
DROP INDEX IF EXISTS idx_customer_rooms_floor_id;
DROP INDEX IF EXISTS idx_assets_customer_id;
DROP INDEX IF EXISTS idx_as_receipts_customer_id;
-- idx_as_processes_as_id 는 041 과 공유하므로 049 롤백에서 떼지 않는다.

DELETE FROM schema_migrations WHERE version = 49;
