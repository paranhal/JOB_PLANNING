-- 053 롤백: 표시 이름만 되돌린다. stage 코드값은 건드리지 않는다.
UPDATE codes SET code_name='담당자 접촉' WHERE code_id='SST02' AND code_value='contact';
UPDATE codes SET code_name='정보 입수' WHERE code_id='SST01' AND code_value='lead';
UPDATE codes SET code_name='제안 진행' WHERE code_id='SST03' AND code_value='proposal';
