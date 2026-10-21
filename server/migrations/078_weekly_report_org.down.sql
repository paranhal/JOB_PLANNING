-- 95-10 down. company_weekly_reports 와 activity_kind·sort_order 만 되돌린다. org_id(92)는 유지.

DROP INDEX IF EXISTS ix_cwr_org_week;
DROP TABLE IF EXISTS company_weekly_reports;
DROP INDEX IF EXISTS ix_weekly_report_rows_org;

CREATE TABLE IF NOT EXISTS weekly_report_rows_078old (
  row_key      TEXT PRIMARY KEY,
  sheet_row    INTEGER NOT NULL,
  division     TEXT NOT NULL DEFAULT '',
  team         TEXT NOT NULL DEFAULT '',
  no_label     TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT '',
  project_id   TEXT,
  row_kind     TEXT NOT NULL DEFAULT 'project',
  highlight    INTEGER NOT NULL DEFAULT 0,
  is_active    INTEGER NOT NULL DEFAULT 1,
  org_id       TEXT NOT NULL DEFAULT ''
);

INSERT INTO weekly_report_rows_078old
  (row_key, sheet_row, division, team, no_label, display_name, project_id, row_kind, highlight, is_active, org_id)
SELECT row_key, sheet_row, COALESCE(division,''), COALESCE(team,''), COALESCE(no_label,''),
       COALESCE(display_name,''), project_id, COALESCE(row_kind,'project'),
       COALESCE(highlight,0), COALESCE(is_active,1), COALESCE(org_id,'')
FROM weekly_report_rows;

DROP TABLE weekly_report_rows;
ALTER TABLE weekly_report_rows_078old RENAME TO weekly_report_rows;
CREATE INDEX IF NOT EXISTS idx_weekly_report_rows_project ON weekly_report_rows(project_id);
CREATE INDEX IF NOT EXISTS idx_weekly_report_rows_sheet ON weekly_report_rows(sheet_row);
