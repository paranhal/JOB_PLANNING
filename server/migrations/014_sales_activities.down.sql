-- 014 롤백. 영업 활동 표와 유형 코드만 제거한다.
-- 연결된 work_tasks(source_type='sales_activity') 도 함께 지운다.

DELETE FROM work_tasks WHERE source_type='sales_activity';
DROP TABLE IF EXISTS sales_activities;

DELETE FROM codes WHERE code_group='sales_activity_type';
DELETE FROM id_sequences WHERE seq_key IN (
  'sales_activity',
  '__meta:sales_activities_v1',
  '__meta:sales_activity_type_codes_v1'
);
