-- 031 롤백. 8단계 codes·stage 를 되돌린다.
-- 컬럼(legacy_stage/won_at/contracted_at)은 SQLite 에서 떼지 않는다.
-- sales_stage_history 는 원래부터 옛 코드를 유지하므로 건드리지 않는다.

UPDATE codes SET code_name='정보 입수', sort_order=1, is_active=1 WHERE code_id='SST01';
UPDATE codes SET code_name='담당자 접촉', sort_order=2, is_active=1 WHERE code_id='SST02';
UPDATE codes SET code_name='제안 진행', sort_order=3, is_active=1 WHERE code_id='SST03';
UPDATE codes SET code_name='견적 요청', sort_order=4, is_active=1 WHERE code_id='SST04';
UPDATE codes SET code_name='RFP 제안', sort_order=5, is_active=1 WHERE code_id='SST05';
UPDATE codes SET code_name='제안서 제출', sort_order=6, is_active=1 WHERE code_id='SST06';
UPDATE codes SET code_name='수주확정', sort_order=7, is_active=1 WHERE code_id='SST07';
UPDATE codes SET code_name='계약완료', sort_order=8, is_active=1 WHERE code_id='SST08';
UPDATE codes SET code_name='수주실패', sort_order=9, is_active=1 WHERE code_id='SST09';
UPDATE codes SET is_active=0, sort_order=4 WHERE code_id='SST10';

UPDATE codes SET code_name='10', sort_order=1, is_active=1 WHERE code_id='SSP01';
UPDATE codes SET code_name='10', sort_order=2, is_active=1 WHERE code_id='SSP02';
UPDATE codes SET code_name='15', sort_order=3, is_active=1 WHERE code_id='SSP03';
UPDATE codes SET code_name='20', sort_order=4, is_active=1 WHERE code_id='SSP04';
UPDATE codes SET code_name='30', sort_order=5, is_active=1 WHERE code_id='SSP05';
UPDATE codes SET code_name='50', sort_order=6, is_active=1 WHERE code_id='SSP06';
UPDATE codes SET code_name='90', sort_order=7, is_active=1 WHERE code_id='SSP07';
UPDATE codes SET code_name='100', sort_order=8, is_active=1 WHERE code_id='SSP08';
UPDATE codes SET code_name='0', sort_order=9, is_active=1 WHERE code_id='SSP09';
UPDATE codes SET is_active=0, sort_order=4 WHERE code_id='SSP10';

UPDATE codes SET is_active=0 WHERE code_id='SAT11';

UPDATE sales_projects SET stage = legacy_stage
 WHERE TRIM(COALESCE(legacy_stage,'')) != '';

UPDATE sales_projects SET probability = CASE stage
  WHEN 'lead' THEN 10
  WHEN 'contact' THEN 10
  WHEN 'proposal' THEN 15
  WHEN 'quote' THEN 20
  WHEN 'rfp' THEN 30
  WHEN 'submit' THEN 50
  WHEN 'won' THEN 90
  WHEN 'contracted' THEN 100
  WHEN 'lost' THEN 0
  ELSE probability
END
WHERE probability_override IS NULL;

DELETE FROM sales_activities WHERE title LIKE '[이관]%';
