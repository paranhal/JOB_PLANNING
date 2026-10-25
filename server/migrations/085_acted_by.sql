-- 95-36 대체 입력자. as_processes 는 이미 있음. 업무·영업활동·점검 방문에 동일 개념.

ALTER TABLE work_tasks ADD COLUMN acted_by_user_id TEXT;
ALTER TABLE work_tasks ADD COLUMN acted_by_name TEXT;
ALTER TABLE work_tasks ADD COLUMN on_behalf INTEGER NOT NULL DEFAULT 0;

ALTER TABLE sales_activities ADD COLUMN acted_by_user_id TEXT;
ALTER TABLE sales_activities ADD COLUMN acted_by_name TEXT;
ALTER TABLE sales_activities ADD COLUMN on_behalf INTEGER NOT NULL DEFAULT 0;

ALTER TABLE maintenance_visits ADD COLUMN acted_by_user_id TEXT;
ALTER TABLE maintenance_visits ADD COLUMN acted_by_name TEXT;
ALTER TABLE maintenance_visits ADD COLUMN on_behalf INTEGER NOT NULL DEFAULT 0;
