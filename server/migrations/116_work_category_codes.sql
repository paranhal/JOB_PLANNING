-- 95-224: 업무 구분 7종을 코드 관리(work_category)로 옮긴다. code_value 는 work_tasks.activity_kind 키와 같다. 멱등.
-- 기술영업(tech)→영업, 고객서비스(service)→사업 부문으로 이름을 바꿨다(키는 그대로).
INSERT INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active)
SELECT 'CD-WC-' || v.code_value, 'work_category', v.code_value, v.code_name, v.sort_order, 1
  FROM (SELECT 'mgmt' AS code_value, '경영지원' AS code_name, 10 AS sort_order
        UNION ALL SELECT 'asset', '자산관리', 20
        UNION ALL SELECT 'tech', '영업', 30
        UNION ALL SELECT 'strategy', '전략및혁신', 40
        UNION ALL SELECT 'service', '사업 부문', 50
        UNION ALL SELECT 'biz_support', '업무지원', 60
        UNION ALL SELECT 'growth', '역량강화', 70) v
 WHERE NOT EXISTS (SELECT 1 FROM codes c WHERE c.code_group='work_category' AND c.code_value=v.code_value);
