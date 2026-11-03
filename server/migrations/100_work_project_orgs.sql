-- 95-87 유관 부서. 주관은 work_projects.org_id. share_to 있는 공개만 백필.

CREATE TABLE IF NOT EXISTS work_project_orgs (
  project_id TEXT NOT NULL,
  org_id TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'related',
  created_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
  created_by TEXT,
  PRIMARY KEY (project_id, org_id)
);
CREATE INDEX IF NOT EXISTS idx_work_project_orgs_org ON work_project_orgs(org_id);

INSERT OR IGNORE INTO work_project_orgs (project_id, org_id, role, created_by)
SELECT p.project_id, TRIM(p.share_to), 'related', 'migrate:100'
  FROM work_projects p
 WHERE COALESCE(p.is_shared,0)=1
   AND TRIM(COALESCE(p.share_to,'')) != ''
   AND TRIM(COALESCE(p.share_to,'')) != TRIM(COALESCE(p.org_id,''));
