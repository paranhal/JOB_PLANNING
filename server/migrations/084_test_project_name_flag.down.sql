-- 084 되돌리기. 이름으로 올린 플래그만 0으로 돌린다. 원래 is_test=1 이던 (TEST) 사업은 구분하지 못한다.

UPDATE work_tasks
   SET is_test = 0
 WHERE COALESCE(is_test, 0) = 1
   AND TRIM(COALESCE(project_id, '')) != ''
   AND EXISTS (
        SELECT 1 FROM work_projects p
         WHERE p.project_id = work_tasks.project_id
           AND p.name LIKE '(TEST)%'
           AND TRIM(COALESCE(p.org_id, '')) != ''
           AND TRIM(COALESCE(work_tasks.org_id, '')) != ''
           AND TRIM(work_tasks.org_id) != TRIM(p.org_id)
   );

UPDATE work_projects
   SET is_test = 0
 WHERE name LIKE '(TEST)%';
