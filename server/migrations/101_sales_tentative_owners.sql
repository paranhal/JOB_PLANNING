-- 95-88 가칭 저장·영업 담당 다중화

ALTER TABLE sales_projects ADD COLUMN tentative_name TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS sales_project_owners (
  sales_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  is_primary INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
  PRIMARY KEY (sales_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_sales_project_owners_user ON sales_project_owners(user_id);

INSERT OR IGNORE INTO sales_project_owners (sales_id, user_id, is_primary, created_at)
SELECT sales_id, TRIM(sales_owner_id), 1, datetime('now','localtime')
  FROM sales_projects
 WHERE TRIM(COALESCE(sales_owner_id,'')) != '';

UPDATE sales_projects
   SET tentative_name = name
 WHERE COALESCE(is_tentative_name,0)=1
   AND TRIM(COALESCE(tentative_name,'')) = ''
   AND TRIM(COALESCE(name,'')) != '';
