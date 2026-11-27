-- 95-224 매칭 되돌리기: 이 마이그레이션이 바꾼 업무만 이전 값으로 복원(그 사이 사용자가 바꾼 업무는 건드리지 않는다)
UPDATE work_tasks SET
  project_id=(SELECT before_project FROM work_task_match_115 x WHERE x.task_id=work_tasks.task_id),
  activity_kind=(SELECT before_activity FROM work_task_match_115 x WHERE x.task_id=work_tasks.task_id)
 WHERE task_id IN (SELECT task_id FROM work_task_match_115 x WHERE x.after_project IS work_tasks.project_id AND x.after_activity IS work_tasks.activity_kind);
DROP TABLE IF EXISTS work_task_match_115;
DELETE FROM app_settings WHERE setting_key='migration_115_done';
