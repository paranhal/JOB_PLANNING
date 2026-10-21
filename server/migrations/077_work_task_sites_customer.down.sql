-- 95-9A down: work_task_sites 를 (task_id, site_id) 로 되돌린다.

CREATE TABLE IF NOT EXISTS work_task_sites_old (
  task_id TEXT NOT NULL,
  site_id TEXT NOT NULL,
  PRIMARY KEY (task_id, site_id)
);
CREATE INDEX IF NOT EXISTS ix_wts_site ON work_task_sites_old(site_id);

INSERT OR IGNORE INTO work_task_sites_old(task_id, site_id)
SELECT task_id, COALESCE(NULLIF(TRIM(project_site_id),''), customer_id)
FROM work_task_sites
WHERE TRIM(COALESCE(NULLIF(TRIM(project_site_id),''), customer_id)) != '';

DROP INDEX IF EXISTS ix_wts_customer;
DROP INDEX IF EXISTS ix_wts_psite;
DROP TABLE work_task_sites;
ALTER TABLE work_task_sites_old RENAME TO work_task_sites;
