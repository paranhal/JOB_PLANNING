-- 91단계 D-97. 활동 유형(내용)과 방법 분리.

ALTER TABLE sales_activities ADD COLUMN activity_method TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS sales_activities_pre91 (
  activity_id TEXT PRIMARY KEY,
  activity_type TEXT NOT NULL DEFAULT '',
  captured_at TEXT NOT NULL DEFAULT ''
);

INSERT OR IGNORE INTO sales_activities_pre91 (activity_id, activity_type, captured_at)
SELECT activity_id, COALESCE(activity_type,''), datetime('now') FROM sales_activities;

INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active) VALUES
  ('SAM01','sales_activity_method','visit','방문',1,1),
  ('SAM02','sales_activity_method','call','전화',2,1),
  ('SAM03','sales_activity_method','mail','이메일',3,1),
  ('SAM04','sales_activity_method','online','온라인미팅',4,1),
  ('SAM05','sales_activity_method','internal','내부회의',5,1),
  ('SAM06','sales_activity_method','other','기타',6,1);

UPDATE codes SET code_name='정보 수집', sort_order=1, is_active=1 WHERE code_group='sales_activity_type' AND code_value='research';
UPDATE codes SET code_name='자료송부', sort_order=2, is_active=1 WHERE code_group='sales_activity_type' AND code_value='material';
UPDATE codes SET code_name='견적제출', sort_order=3, is_active=1 WHERE code_group='sales_activity_type' AND code_value='quote';
UPDATE codes SET code_name='요구사항 파악', sort_order=4, is_active=1 WHERE code_group='sales_activity_type' AND code_value='requirement';
UPDATE codes SET code_name='RFP 접수', sort_order=5, is_active=1 WHERE code_group='sales_activity_type' AND code_value='rfp_received';
UPDATE codes SET code_name='제안서 제출', sort_order=6, is_active=1 WHERE code_group='sales_activity_type' AND code_value='proposal';
UPDATE codes SET code_name='기타', sort_order=7, is_active=1 WHERE code_group='sales_activity_type' AND code_value='other';

UPDATE codes SET is_active=0 WHERE code_group='sales_activity_type' AND code_value IN ('visit','call','mail','online','internal','rfp','bid');

UPDATE sales_activities SET activity_type='other', activity_method=activity_type
WHERE TRIM(COALESCE(activity_method,''))='' AND activity_type IN ('visit','call','mail','online','internal');

UPDATE sales_activities SET activity_type='proposal', activity_method=''
WHERE TRIM(COALESCE(activity_method,''))='' AND activity_type='rfp';

UPDATE sales_activities SET activity_type='other', activity_method=''
WHERE TRIM(COALESCE(activity_method,''))='' AND activity_type='bid';
