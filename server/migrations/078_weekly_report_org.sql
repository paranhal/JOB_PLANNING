-- 95-10 주간보고 행 조직화. 시드 INSERT 없음(O02/O03 행은 화면에서 등록).
-- weekly_report_rows.org_id 는 92 org 마이그레이션에 있을 수 있음(중복 ADD 무시).

ALTER TABLE weekly_report_rows ADD COLUMN org_id TEXT NOT NULL DEFAULT '';
ALTER TABLE weekly_report_rows ADD COLUMN activity_kind TEXT NOT NULL DEFAULT '';
ALTER TABLE weekly_report_rows ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;

UPDATE weekly_report_rows
   SET org_id = 'O01'
 WHERE TRIM(COALESCE(org_id,'')) = '';

UPDATE weekly_report_rows
   SET activity_kind = 'asset'
 WHERE row_key = 'X02'
    OR (COALESCE(row_kind,'') = 'activity' AND display_name LIKE '%자산관리%');

UPDATE weekly_report_rows
   SET activity_kind = 'tech'
 WHERE row_key = 'X03'
    OR (COALESCE(row_kind,'') = 'activity' AND display_name LIKE '%기술영업%');

UPDATE weekly_report_rows
   SET sort_order = CASE WHEN sheet_row >= 20 THEN sheet_row - 20 ELSE 0 END
 WHERE COALESCE(sort_order,0) = 0 AND sheet_row > 0;

CREATE INDEX IF NOT EXISTS ix_weekly_report_rows_org ON weekly_report_rows(org_id, sort_order);

CREATE TABLE IF NOT EXISTS company_weekly_reports (
  report_id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL,
  week_anchor TEXT NOT NULL,
  sheet_name TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'draft',
  content_json TEXT NOT NULL DEFAULT '',
  confirmed_by TEXT NOT NULL DEFAULT '',
  confirmed_at TEXT NOT NULL DEFAULT '',
  updated_by TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  UNIQUE(org_id, week_anchor)
);
CREATE INDEX IF NOT EXISTS ix_cwr_org_week ON company_weekly_reports(org_id, week_anchor);
