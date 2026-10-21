-- 95-9 일일 업무 「작업」(구축·공사). 시드 INSERT 없음.

ALTER TABLE work_tasks ADD COLUMN work_subtype TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS work_task_sites (
  task_id TEXT NOT NULL,
  site_id TEXT NOT NULL,
  PRIMARY KEY (task_id, site_id)
);
CREATE INDEX IF NOT EXISTS ix_wts_site ON work_task_sites(site_id);
