-- 012 롤백. 영업 필드와 코드 매핑을 제거한다.
-- SQLite 3.35+ 의 DROP COLUMN 을 쓴다.

DELETE FROM codes WHERE code_group IN ('sales_stage','sales_lead_source');
ALTER TABLE work_projects DROP COLUMN sales_stage;
ALTER TABLE work_projects DROP COLUMN expected_ym;
ALTER TABLE work_projects DROP COLUMN expected_precision;
ALTER TABLE work_projects DROP COLUMN expected_note;
ALTER TABLE work_projects DROP COLUMN expected_undated_reason;
ALTER TABLE work_projects DROP COLUMN prospect_name;
ALTER TABLE work_projects DROP COLUMN prospect_region;
ALTER TABLE work_projects DROP COLUMN prospect_contact_name;
ALTER TABLE work_projects DROP COLUMN prospect_contact_title;
ALTER TABLE work_projects DROP COLUMN prospect_contact_phone;
ALTER TABLE work_projects DROP COLUMN prospect_contact_email;
ALTER TABLE work_projects DROP COLUMN sales_owner;
ALTER TABLE work_projects DROP COLUMN sales_owner_id;
ALTER TABLE work_projects DROP COLUMN expected_amount;
ALTER TABLE work_projects DROP COLUMN win_probability;
ALTER TABLE work_projects DROP COLUMN competitor;
ALTER TABLE work_projects DROP COLUMN lead_source;
