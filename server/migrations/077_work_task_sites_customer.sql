-- 95-9A 작업 대상 사이트를 고객(customer_id) 기준으로 재구성. 시드 INSERT 없음.
-- 기존 행은 project_sites.customer_id 가 있는 건만 이관한다.

CREATE TABLE IF NOT EXISTS work_task_sites_new (
  task_id TEXT NOT NULL,
  customer_id TEXT NOT NULL,
  project_site_id TEXT,
  PRIMARY KEY (task_id, customer_id)
);
CREATE INDEX IF NOT EXISTS ix_wts_customer ON work_task_sites_new(customer_id);
CREATE INDEX IF NOT EXISTS ix_wts_psite ON work_task_sites_new(project_site_id);

INSERT INTO work_task_sites_new(task_id, customer_id, project_site_id)
SELECT w.task_id, s.customer_id, w.site_id
FROM work_task_sites w
JOIN project_sites s ON s.id = w.site_id
WHERE TRIM(COALESCE(s.customer_id,'')) != '';

DROP INDEX IF EXISTS ix_wts_site;
DROP TABLE work_task_sites;
ALTER TABLE work_task_sites_new RENAME TO work_task_sites;
