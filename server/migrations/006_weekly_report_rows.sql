-- 기획서 §16.6.5
-- 전사 주간업무보고 엑셀 행 ↔ 사업 대응표. 사업은 매년 바뀌므로 코드에 박지 않는다.
-- 실제 앱은 InitDB 시 repository.applyWeeklyReportRows 가 동일 SQL을 실행한 뒤 초기 6행을 넣는다.

CREATE TABLE IF NOT EXISTS weekly_report_rows (
  row_key      TEXT PRIMARY KEY,
  sheet_row    INTEGER NOT NULL,
  division     TEXT NOT NULL DEFAULT '',
  team         TEXT NOT NULL DEFAULT '',
  no_label     TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT '',
  project_id   TEXT,
  row_kind     TEXT NOT NULL DEFAULT 'project',
  highlight    INTEGER NOT NULL DEFAULT 0,
  is_active    INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_weekly_report_rows_project ON weekly_report_rows(project_id);
CREATE INDEX IF NOT EXISTS idx_weekly_report_rows_sheet ON weekly_report_rows(sheet_row);
