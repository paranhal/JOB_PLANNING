-- 기획서 §22.1.1 ②~⑦ · ⑪단계 2
-- 영업 단계·예정월·가등록 고객·영업담당·금액·확도.
-- 실제 앱은 InitDB 시 repository.applySalesProject 가 동일 SQL을 실행한다.

ALTER TABLE work_projects ADD COLUMN sales_stage TEXT;
ALTER TABLE work_projects ADD COLUMN expected_ym TEXT;
ALTER TABLE work_projects ADD COLUMN expected_precision TEXT NOT NULL DEFAULT 'month';
ALTER TABLE work_projects ADD COLUMN expected_note TEXT;
ALTER TABLE work_projects ADD COLUMN expected_undated_reason TEXT;
ALTER TABLE work_projects ADD COLUMN prospect_name TEXT;
ALTER TABLE work_projects ADD COLUMN prospect_region TEXT;
ALTER TABLE work_projects ADD COLUMN prospect_contact_name TEXT;
ALTER TABLE work_projects ADD COLUMN prospect_contact_title TEXT;
ALTER TABLE work_projects ADD COLUMN prospect_contact_phone TEXT;
ALTER TABLE work_projects ADD COLUMN prospect_contact_email TEXT;
ALTER TABLE work_projects ADD COLUMN sales_owner TEXT;
ALTER TABLE work_projects ADD COLUMN sales_owner_id TEXT;
ALTER TABLE work_projects ADD COLUMN expected_amount INTEGER;
ALTER TABLE work_projects ADD COLUMN win_probability INTEGER;
ALTER TABLE work_projects ADD COLUMN competitor TEXT;
ALTER TABLE work_projects ADD COLUMN lead_source TEXT;

INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order) VALUES
  ('SS001','sales_stage','lead','발굴',1),
  ('SS002','sales_stage','proposal','제안',2),
  ('SS003','sales_stage','quote','견적',3),
  ('SS004','sales_stage','bid','입찰·협상',4),
  ('SS005','sales_stage','won','수주',5),
  ('SS006','sales_stage','lost','실주',6),
  ('SS007','sales_stage','dropped','보류',7),
  ('SLS001','sales_lead_source','existing','기존 고객',1),
  ('SLS002','sales_lead_source','bid','입찰공고',2),
  ('SLS003','sales_lead_source','referral','소개',3),
  ('SLS004','sales_lead_source','exhibition','전시회',4),
  ('SLS005','sales_lead_source','other','기타',5);
