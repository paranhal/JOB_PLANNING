-- 95-227: 접수·방문의 사업을 그 원본에서 나온 업무로 복사(기존 건 보정). 업무에 사업이 비어 있는 건만 채우고 덮어쓰지 않는다.
-- 사업과 업무 구분은 함께 둘 수 없으므로 채운 업무의 activity_kind 는 비운다. 변경 전후는 work_task_copy_117 에 남긴다(되돌리기용).
CREATE TABLE IF NOT EXISTS work_task_copy_117 (
  task_id TEXT PRIMARY KEY, source_type TEXT, source_id TEXT, before_project TEXT, before_activity TEXT,
  after_project TEXT, applied_at TEXT NOT NULL DEFAULT (datetime('now','localtime')));
INSERT OR IGNORE INTO work_task_copy_117 (task_id, source_type, source_id, before_project, before_activity, after_project)
SELECT t.task_id, t.source_type, t.source_id, t.project_id, t.activity_kind, TRIM(ar.project_id)
  FROM work_tasks t JOIN as_receipts ar ON ar.as_id=t.source_id
 WHERE t.source_type='as' AND COALESCE(TRIM(t.project_id),'')='' AND COALESCE(TRIM(ar.project_id),'')<>'';
INSERT OR IGNORE INTO work_task_copy_117 (task_id, source_type, source_id, before_project, before_activity, after_project)
SELECT t.task_id, t.source_type, t.source_id, t.project_id, t.activity_kind, TRIM(ar.project_id)
  FROM work_tasks t JOIN as_work_items w ON w.work_id=t.source_id JOIN as_receipts ar ON ar.as_id=w.as_id
 WHERE t.source_type='as_work' AND COALESCE(TRIM(t.project_id),'')='' AND COALESCE(TRIM(ar.project_id),'')<>'';
INSERT OR IGNORE INTO work_task_copy_117 (task_id, source_type, source_id, before_project, before_activity, after_project)
SELECT t.task_id, t.source_type, t.source_id, t.project_id, t.activity_kind, TRIM(mv.project_id)
  FROM work_tasks t JOIN maintenance_visits mv ON mv.visit_id=t.source_id
 WHERE t.source_type='maintenance' AND COALESCE(TRIM(t.project_id),'')='' AND COALESCE(TRIM(mv.project_id),'')<>'';
UPDATE work_tasks SET
  project_id=(SELECT after_project FROM work_task_copy_117 x WHERE x.task_id=work_tasks.task_id),
  activity_kind=NULL, updated_at=datetime('now','localtime')
 WHERE task_id IN (SELECT task_id FROM work_task_copy_117) AND COALESCE(TRIM(project_id),'')='';
