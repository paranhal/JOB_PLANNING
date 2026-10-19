-- 82-A D-83. 사업 유형 확정 여부. 기존 행은 0(미확정) 그대로, project_kind 값은 바꾸지 않는다.

ALTER TABLE work_projects ADD COLUMN project_kind_set INTEGER NOT NULL DEFAULT 0;
