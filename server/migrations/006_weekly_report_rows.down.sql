-- 006 롤백. 대응표만 제거한다. 업무 원본(방문·접수·일일업무)은 건드리지 않는다.

DROP INDEX IF EXISTS idx_weekly_report_rows_sheet;
DROP INDEX IF EXISTS idx_weekly_report_rows_project;
DROP TABLE IF EXISTS weekly_report_rows;
