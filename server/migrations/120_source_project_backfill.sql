-- 95-230: 95-224 매칭으로 업무(work_tasks)에만 들어간 사업을 그 원본(접수·점검 방문)에 복사한다. 원본이 진실이다(95-227).
-- 비어 있는 접수·방문만 채우고 덮어쓰지 않는다. 한 접수·방문에서 나온 업무들의 사업이 서로 다르면 건드리지 않는다.
-- 변경 전후는 source_project_backfill_120 에 남긴다(되돌리기용).
CREATE TABLE IF NOT EXISTS source_project_backfill_120 (
  source_type TEXT NOT NULL, source_id TEXT NOT NULL, before_project TEXT, after_project TEXT NOT NULL,
  applied_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
  PRIMARY KEY (source_type, source_id));

-- AS 접수(접수 업무 + 하부업무 업무 모두 같은 사업일 때)
INSERT OR IGNORE INTO source_project_backfill_120 (source_type, source_id, before_project, after_project)
SELECT 'as', ar.as_id, ar.project_id, MIN(t.project_id)
  FROM as_receipts ar
  JOIN work_tasks t ON (t.source_type='as' AND t.source_id=ar.as_id)
                    OR (t.source_type='as_work' AND t.source_id IN (SELECT w.work_id FROM as_work_items w WHERE w.as_id=ar.as_id))
 WHERE COALESCE(TRIM(ar.project_id),'')=''
   AND COALESCE(TRIM(t.project_id),'')<>''
   AND EXISTS (SELECT 1 FROM work_projects p WHERE p.project_id=t.project_id)
 GROUP BY ar.as_id
HAVING COUNT(DISTINCT t.project_id)=1;

-- 정기점검 방문
INSERT OR IGNORE INTO source_project_backfill_120 (source_type, source_id, before_project, after_project)
SELECT 'maintenance', mv.visit_id, mv.project_id, MIN(t.project_id)
  FROM maintenance_visits mv
  JOIN work_tasks t ON t.source_type='maintenance' AND t.source_id=mv.visit_id
 WHERE COALESCE(TRIM(mv.project_id),'')=''
   AND COALESCE(TRIM(t.project_id),'')<>''
   AND EXISTS (SELECT 1 FROM work_projects p WHERE p.project_id=t.project_id)
 GROUP BY mv.visit_id
HAVING COUNT(DISTINCT t.project_id)=1;

UPDATE as_receipts SET project_id=(SELECT b.after_project FROM source_project_backfill_120 b WHERE b.source_type='as' AND b.source_id=as_receipts.as_id)
 WHERE COALESCE(TRIM(project_id),'')='' AND as_id IN (SELECT source_id FROM source_project_backfill_120 WHERE source_type='as');
UPDATE maintenance_visits SET project_id=(SELECT b.after_project FROM source_project_backfill_120 b WHERE b.source_type='maintenance' AND b.source_id=maintenance_visits.visit_id)
 WHERE COALESCE(TRIM(project_id),'')='' AND visit_id IN (SELECT source_id FROM source_project_backfill_120 WHERE source_type='maintenance');
