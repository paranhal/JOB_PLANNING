-- 053 영업 단계 표시 이름만 변경. 코드값·확도·이력은 그대로. §46.2
UPDATE codes SET code_name='발굴' WHERE code_id='SST02' AND code_value='contact';
UPDATE codes SET code_name='검토' WHERE code_id='SST01' AND code_value='lead';
UPDATE codes SET code_name='견적' WHERE code_id='SST03' AND code_value='proposal';
