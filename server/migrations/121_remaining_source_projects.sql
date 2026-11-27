-- 95-231: 남은 사업 미연결 접수 정리(사용자 확정 방침).
--  ① 최혜영 담당 충남 도서관 접수(금산·당진) → 사업 WPSEED01 로 연결(접수 + 그 접수의 업무).
--  ② 사업 사이트에 없는 고객(비젼아이티·AI DATA·채움씨앤아이) 접수 15건의 업무 → 업무 구분 「사업 부문」(service).
-- 비어 있는 건만 채우고 덮어쓰지 않는다. 변경 전후는 source_remaining_backfill_121 에 남긴다(되돌리기용).
CREATE TABLE IF NOT EXISTS source_remaining_backfill_121 (
  kind TEXT NOT NULL, ref_id TEXT NOT NULL, before_value TEXT, after_value TEXT NOT NULL,
  applied_at TEXT NOT NULL DEFAULT (datetime('now','localtime')), PRIMARY KEY (kind, ref_id));

INSERT OR IGNORE INTO source_remaining_backfill_121 (kind, ref_id, before_value, after_value)
SELECT 'receipt_project', ar.as_id, ar.project_id, 'WPSEED01' FROM as_receipts ar
 WHERE ar.as_number IN ('R2611-108','R2611-109') AND COALESCE(TRIM(ar.project_id),'')=''
   AND EXISTS (SELECT 1 FROM work_projects p WHERE p.project_id='WPSEED01');
UPDATE as_receipts SET project_id='WPSEED01' WHERE as_id IN (SELECT ref_id FROM source_remaining_backfill_121 WHERE kind='receipt_project') AND COALESCE(TRIM(project_id),'')='';

INSERT OR IGNORE INTO source_remaining_backfill_121 (kind, ref_id, before_value, after_value)
SELECT 'task_project', t.task_id, t.project_id, 'WPSEED01' FROM work_tasks t
 WHERE COALESCE(TRIM(t.project_id),'')='' AND (
       (t.source_type='as' AND t.source_id IN (SELECT ref_id FROM source_remaining_backfill_121 WHERE kind='receipt_project'))
    OR (t.source_type='as_work' AND t.source_id IN (SELECT w.work_id FROM as_work_items w WHERE w.as_id IN (SELECT ref_id FROM source_remaining_backfill_121 WHERE kind='receipt_project'))));
UPDATE work_tasks SET project_id='WPSEED01', activity_kind=NULL, updated_at=datetime('now','localtime')
 WHERE task_id IN (SELECT ref_id FROM source_remaining_backfill_121 WHERE kind='task_project') AND COALESCE(TRIM(project_id),'')='';

INSERT OR IGNORE INTO source_remaining_backfill_121 (kind, ref_id, before_value, after_value)
SELECT 'task_activity', t.task_id, t.activity_kind, 'service' FROM work_tasks t
  JOIN as_receipts ar ON (t.source_type='as' AND t.source_id=ar.as_id)
                      OR (t.source_type='as_work' AND t.source_id IN (SELECT w.work_id FROM as_work_items w WHERE w.as_id=ar.as_id))
 WHERE ar.as_number IN ('R2607-062','R2607-063','R2607-090','R2607-091','R2607-093','R2607-101','R2607-102','R2607-134','R2607-135','R2607-219',
                        'R2607-046','R2607-047','R2607-058','R2607-059','R2607-107')
   AND COALESCE(TRIM(ar.project_id),'')='' AND COALESCE(TRIM(t.project_id),'')='' AND COALESCE(TRIM(t.activity_kind),'')='';
UPDATE work_tasks SET activity_kind='service', updated_at=datetime('now','localtime')
 WHERE task_id IN (SELECT ref_id FROM source_remaining_backfill_121 WHERE kind='task_activity') AND COALESCE(TRIM(project_id),'')='' AND COALESCE(TRIM(activity_kind),'')='';
