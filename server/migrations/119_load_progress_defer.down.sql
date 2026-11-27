-- 95-226 되돌리기: 일자별 저장소 제거. ADD COLUMN 으로 추가한 컬럼은 SQLite 제약상 남긴다(값은 비어 있어도 무해).
DROP INDEX IF EXISTS idx_wtdp_task;
DROP TABLE IF EXISTS work_task_day_progress;
