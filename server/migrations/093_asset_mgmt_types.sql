-- 093 설치자산 관리유형 3종 · 미계약 점검주기 CALL (95-54)
-- 데이터 UPDATE 는 멱등. 리포트 CSV 는 cmd/migrate093 이 만든다.
-- down SQL 로 자산 값을 되돌릴 수 없다. 적용 전 백업을 복원한다.

UPDATE codes SET is_active=0
 WHERE code_group='management_type'
   AND code_value IN ('direct','on_demand','reference');

UPDATE codes SET sort_order=1, is_active=1, code_name='장애대응'
 WHERE code_group='management_type' AND code_value='fault';
UPDATE codes SET sort_order=2, is_active=1, code_name='정기점검'
 WHERE code_group='management_type' AND code_value='periodic';
UPDATE codes SET sort_order=3, is_active=1, code_name='타사장비'
 WHERE code_group='management_type' AND code_value='third_party';

INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active)
VALUES ('MCY007','maint_cycle','call','CALL',6,1);
UPDATE codes SET sort_order=6, is_active=1, code_name='CALL'
 WHERE code_group='maint_cycle' AND code_value='call';
UPDATE codes SET sort_order=7
 WHERE code_group='maint_cycle' AND code_value='custom';

UPDATE assets SET management_type='periodic'
 WHERE LOWER(TRIM(COALESCE(management_type,''))) IN ('direct','on_demand','reference');

UPDATE assets SET management_type='periodic'
 WHERE TRIM(COALESCE(management_type,''))=''
   AND LOWER(TRIM(COALESCE(maint_contract_type,''))) IN ('paid','free');

UPDATE assets SET management_type='fault'
 WHERE TRIM(COALESCE(management_type,''))=''
   AND LOWER(TRIM(COALESCE(maint_contract_type,'')))='none';

UPDATE assets SET maint_cycle='call'
 WHERE LOWER(TRIM(COALESCE(maint_contract_type,'')))='none'
   AND TRIM(COALESCE(maint_cycle,''))<>'';
