-- 95-43: AS·방문·영업에서 만든 일일 업무의 org_id 를 출처와 맞춘다. 멱등.

UPDATE work_tasks
   SET org_id = (
        SELECT ar.org_id FROM as_receipts ar WHERE ar.as_id = work_tasks.source_id
       )
 WHERE source_type = 'as'
   AND TRIM(COALESCE(source_id,'')) != ''
   AND source_id IN (SELECT as_id FROM as_receipts)
   AND TRIM(COALESCE(org_id,'')) != (
        SELECT TRIM(COALESCE(ar.org_id,'')) FROM as_receipts ar WHERE ar.as_id = work_tasks.source_id
       );

UPDATE work_tasks
   SET org_id = (
        SELECT COALESCE(NULLIF(TRIM(cfg.org_id),''), NULLIF(TRIM(c.org_id),''))
          FROM maintenance_visits v
          JOIN customers c ON c.customer_id = v.customer_id
          LEFT JOIN maintenance_site_config cfg ON cfg.customer_id = v.customer_id
         WHERE v.visit_id = work_tasks.source_id
       )
 WHERE source_type = 'maintenance'
   AND TRIM(COALESCE(source_id,'')) != ''
   AND source_id IN (SELECT visit_id FROM maintenance_visits)
   AND TRIM(COALESCE((
        SELECT COALESCE(NULLIF(TRIM(cfg.org_id),''), NULLIF(TRIM(c.org_id),''))
          FROM maintenance_visits v
          JOIN customers c ON c.customer_id = v.customer_id
          LEFT JOIN maintenance_site_config cfg ON cfg.customer_id = v.customer_id
         WHERE v.visit_id = work_tasks.source_id
       ),'')) != ''
   AND TRIM(COALESCE(org_id,'')) != (
        SELECT TRIM(COALESCE(NULLIF(TRIM(cfg.org_id),''), NULLIF(TRIM(c.org_id),''), ''))
          FROM maintenance_visits v
          JOIN customers c ON c.customer_id = v.customer_id
          LEFT JOIN maintenance_site_config cfg ON cfg.customer_id = v.customer_id
         WHERE v.visit_id = work_tasks.source_id
       );

UPDATE work_tasks
   SET org_id = (
        SELECT p.org_id
          FROM sales_activities a
          JOIN work_projects p ON p.project_id = a.sales_id
         WHERE a.activity_id = work_tasks.source_id
       )
 WHERE source_type = 'sales_activity'
   AND TRIM(COALESCE(source_id,'')) != ''
   AND source_id IN (SELECT activity_id FROM sales_activities)
   AND TRIM(COALESCE((
        SELECT TRIM(COALESCE(p.org_id,''))
          FROM sales_activities a
          JOIN work_projects p ON p.project_id = a.sales_id
         WHERE a.activity_id = work_tasks.source_id
       ),'')) != ''
   AND TRIM(COALESCE(org_id,'')) != (
        SELECT TRIM(COALESCE(p.org_id,''))
          FROM sales_activities a
          JOIN work_projects p ON p.project_id = a.sales_id
         WHERE a.activity_id = work_tasks.source_id
       );
