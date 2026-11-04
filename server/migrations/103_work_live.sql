-- 95-101 실시간 작업 구간 · 이동 · 다음 조치일. 시드 INSERT 없음.

ALTER TABLE work_tasks ADD COLUMN est_minutes INTEGER;
ALTER TABLE work_tasks ADD COLUMN next_action_date TEXT NOT NULL DEFAULT '';
ALTER TABLE work_tasks ADD COLUMN completion_condition TEXT NOT NULL DEFAULT '';

ALTER TABLE work_task_segments ADD COLUMN source TEXT NOT NULL DEFAULT 'manual';
ALTER TABLE work_task_segments ADD COLUMN ended_by TEXT NOT NULL DEFAULT '';
ALTER TABLE work_task_segments ADD COLUMN travel_origin TEXT NOT NULL DEFAULT '';
ALTER TABLE work_task_segments ADD COLUMN travel_dest TEXT NOT NULL DEFAULT '';
ALTER TABLE work_task_segments ADD COLUMN travel_plan_depart TEXT NOT NULL DEFAULT '';
ALTER TABLE work_task_segments ADD COLUMN travel_plan_arrive TEXT NOT NULL DEFAULT '';
ALTER TABLE work_task_segments ADD COLUMN travel_actual_depart TEXT NOT NULL DEFAULT '';
ALTER TABLE work_task_segments ADD COLUMN travel_actual_arrive TEXT NOT NULL DEFAULT '';
ALTER TABLE work_task_segments ADD COLUMN travel_source TEXT NOT NULL DEFAULT 'manual';
ALTER TABLE work_task_segments ADD COLUMN travel_unknown_reason TEXT NOT NULL DEFAULT '';

ALTER TABLE work_task_departures ADD COLUMN reason TEXT NOT NULL DEFAULT '';
ALTER TABLE work_task_departures ADD COLUMN next_action_date TEXT NOT NULL DEFAULT '';
ALTER TABLE work_task_departures ADD COLUMN remaining_min INTEGER;
ALTER TABLE work_task_departures ADD COLUMN force_reason TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS ux_wts_open_per_worker
  ON work_task_segments(worker_user_id)
  WHERE kind='actual' AND TRIM(end_time)='' AND TRIM(worker_user_id)!='';
