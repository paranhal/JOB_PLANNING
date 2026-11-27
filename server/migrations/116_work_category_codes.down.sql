-- 95-224 되돌리기: 업무 구분 코드 제거(업무의 activity_kind 값은 그대로 남는다; 코드가 없으면 기본 목록을 쓴다)
DELETE FROM codes WHERE code_group='work_category';
