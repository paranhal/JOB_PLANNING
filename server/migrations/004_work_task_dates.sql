-- 기획서 §13.4 · §28 S-11
-- work_tasks 에 접수일·완료일을 둔다. 통계가 updated_at 을 완료일로 쓰지 않게 한다.
-- 실제 앱은 InitDB 시 repository.applyS11WorkTaskDates 가 동일 SQL을 실행한다.
--
-- receipt_date  : 업무가 들어온 날. 기본값은 등록일(created_at).
-- complete_date : 완료 처리 시 서버가 기록. 이후 담당자·진행률을 고쳐도 밀리지 않는다.

ALTER TABLE work_tasks ADD COLUMN receipt_date TEXT;
ALTER TABLE work_tasks ADD COLUMN complete_date TEXT;

-- 기존 행: 접수일 = 등록일
UPDATE work_tasks
   SET receipt_date = date(created_at)
 WHERE TRIM(COALESCE(receipt_date,'')) = '';

-- 기존 완료 행: 완료일이 비면 종전 대리지표(updated_at)로 채운다.
-- 이후 조회는 COALESCE(complete_date, updated_at) 이므로 빈 값도 안전하다.
UPDATE work_tasks
   SET complete_date = date(updated_at)
 WHERE status = 'complete'
   AND TRIM(COALESCE(complete_date,'')) = '';

CREATE INDEX IF NOT EXISTS idx_work_tasks_receipt_date  ON work_tasks(receipt_date);
CREATE INDEX IF NOT EXISTS idx_work_tasks_complete_date ON work_tasks(complete_date);
