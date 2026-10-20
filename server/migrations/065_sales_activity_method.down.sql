-- 이관 전 유형을 되돌린다. 백업 표는 사용자 결정 전까지 남긴다.

UPDATE sales_activities
SET activity_type = COALESCE((SELECT p.activity_type FROM sales_activities_pre91 p WHERE p.activity_id = sales_activities.activity_id), activity_type),
    activity_method = ''
WHERE EXISTS (SELECT 1 FROM sales_activities_pre91 p WHERE p.activity_id = sales_activities.activity_id);

UPDATE codes SET is_active=1 WHERE code_group='sales_activity_type' AND code_value IN ('visit','call','mail','online','internal','rfp','bid');
DELETE FROM codes WHERE code_group='sales_activity_method';

DROP INDEX IF EXISTS idx_sales_act_method;
