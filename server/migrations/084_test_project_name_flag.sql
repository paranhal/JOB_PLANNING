-- 95-34 (TEST) 로 시작하는 사업을 is_test=1. 그 사업에 묶인 타 조직 업무만 is_test=1.
-- 같은 조직(O01)·O02 업무는 바꾸지 않는다.

UPDATE work_projects
   SET is_test = 1
 WHERE name LIKE '(TEST)%'
   AND COALESCE(is_test, 0) = 0;

UPDATE work_tasks
   SET is_test = 1
 WHERE COALESCE(is_test, 0) = 0
   AND TRIM(COALESCE(project_id, '')) != ''
   AND EXISTS (
        SELECT 1 FROM work_projects p
         WHERE p.project_id = work_tasks.project_id
           AND COALESCE(p.is_test, 0) = 1
           AND TRIM(COALESCE(p.org_id, '')) != ''
           AND TRIM(COALESCE(work_tasks.org_id, '')) != ''
           AND TRIM(work_tasks.org_id) != TRIM(p.org_id)
           AND TRIM(work_tasks.org_id) != 'O02'
   );
