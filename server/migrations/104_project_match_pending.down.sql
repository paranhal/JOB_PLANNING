-- 104 되돌리기: SQLite 는 컬럼 DROP 을 하지 않는다. 값은 0 으로 되돌린다.
UPDATE work_tasks SET project_match_pending=0 WHERE COALESCE(project_match_pending,0)!=0;
