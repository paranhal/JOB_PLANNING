-- 013 롤백. 영업 사업 표와 단계 코드만 제거한다. work_projects 는 건드리지 않는다.

DROP TABLE IF EXISTS sales_stage_history;
DROP TABLE IF EXISTS sales_projects;

DELETE FROM codes WHERE code_group IN ('sales_stage','sales_stage_prob','sales_lead_source');
DELETE FROM id_sequences WHERE seq_key IN (
  'sales_project',
  'sales_stage_history',
  '__meta:sales_projects_v1',
  '__meta:sales_stage_codes_v1'
);
