-- 032 롤백. 내부회의는 비활성. 옛 라벨 복구. SAT11 은 031 과 맞춘다.

UPDATE codes SET is_active=0 WHERE code_id='SAT12';

UPDATE codes SET code_name='정보수집', sort_order=1, is_active=1 WHERE code_id='SAT01';
UPDATE codes SET code_name='전화', sort_order=2, is_active=1 WHERE code_id='SAT02';
UPDATE codes SET code_name='방문미팅', sort_order=3, is_active=1 WHERE code_id='SAT03';
UPDATE codes SET code_name='온라인미팅', sort_order=4, is_active=1 WHERE code_id='SAT04';
UPDATE codes SET code_name='메일', sort_order=5, is_active=1 WHERE code_id='SAT05';
UPDATE codes SET code_name='자료송부', sort_order=6, is_active=1 WHERE code_id='SAT06';
UPDATE codes SET code_name='견적제출', sort_order=7, is_active=1 WHERE code_id='SAT07';
UPDATE codes SET code_name='제안서제출', sort_order=8, is_active=1 WHERE code_id='SAT08';
UPDATE codes SET code_name='입찰', sort_order=9, is_active=1 WHERE code_id='SAT09';
UPDATE codes SET code_name='기타', sort_order=10, is_active=1 WHERE code_id='SAT10';
UPDATE codes SET code_name='제안서제출(RFP)', sort_order=8, is_active=1 WHERE code_id='SAT11';
