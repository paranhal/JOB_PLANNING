-- 033 롤백. SQLite는 이 컬럼을 안전하게 지우기 어렵다. 자동 생성 표시만 내린다.
-- counterparts 글자는 활동 행에 그대로 남는다.

UPDATE sales_parties SET is_auto=0 WHERE COALESCE(is_auto,0)=1;
