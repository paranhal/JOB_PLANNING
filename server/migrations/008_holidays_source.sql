-- 기획서 §23.13.2 · §23.13.3
-- holidays 에 API/수동 구분·마지막 동기화 시각을 붙인다.
-- 실제 앱은 InitDB 시 repository.applyHolidaySource 가 동일 ALTER·시드를 실행한다.

ALTER TABLE holidays ADD COLUMN source TEXT;
ALTER TABLE holidays ADD COLUMN synced_at DATETIME;

UPDATE holidays SET source = 'manual'
 WHERE kind = 'company' AND TRIM(COALESCE(source,'')) = '';

UPDATE holidays SET source = 'api'
 WHERE kind IN ('public', 'substitute') AND TRIM(COALESCE(source,'')) = '';
