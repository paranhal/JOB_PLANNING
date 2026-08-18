-- 기획서 부록 B. DB 정합성 제약 (DDL)
-- 운영 DB 사본(app.db, 2026-08-14)에서 문법·동작을 검증한 SQL을 그대로 둔다.
--
-- 실행 순서 (부록 B.5):
--   1) B.1 컬럼 추가
--   2) B.2 holidays 테이블
--   3) B.5 보정 B-3 → B-2 → B-1
--   4) B.4 트리거 4종
--   5) B.3 UNIQUE 인덱스
-- 실제 앱은 InitDB 시 repository.applyAppendixB 가 동일 순서로 실행한다.

-- ========== B.1 컬럼 추가 (S-1 ~ S-4, S-10) ==========
ALTER TABLE as_receipts        ADD COLUMN data_origin TEXT NOT NULL DEFAULT 'app';
ALTER TABLE as_receipts        ADD COLUMN schedule_no_date_reason TEXT;
ALTER TABLE as_receipts        ADD COLUMN asset_unlinked_reason   TEXT;
ALTER TABLE maintenance_visits ADD COLUMN project_id  TEXT;
ALTER TABLE maintenance_visits ADD COLUMN data_origin TEXT NOT NULL DEFAULT 'app';

-- ========== B.2 휴무일 테이블 신설 (S-9) ==========
CREATE TABLE IF NOT EXISTS holidays (
  holiday_date TEXT PRIMARY KEY,          -- YYYY-MM-DD
  holiday_year INTEGER NOT NULL,
  name         TEXT NOT NULL,
  kind         TEXT NOT NULL DEFAULT 'public',  -- public | company | substitute
  created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_holidays_year ON holidays(holiday_year);

-- ========== B.5 기존 데이터 보정 — B-3 → B-2 → B-1 ==========
-- B-3: 이관 데이터 표시  → 실행 결과 1,124건
UPDATE as_receipts SET data_origin = 'import'
 WHERE TRIM(COALESCE(import_key,'')) <> '';
-- 나머지 50건은 기본값 'app' 유지

-- B-2: start_datetime 백필 (첫 조치 시각)  → 실행 결과 1,128건
UPDATE as_receipts SET start_datetime = (
        SELECT MIN(p.process_datetime) FROM as_processes p WHERE p.as_id = as_receipts.as_id)
 WHERE TRIM(COALESCE(start_datetime,'')) = ''
   AND EXISTS (SELECT 1 FROM as_processes p WHERE p.as_id = as_receipts.as_id);

-- B-1: 예정일 없이 확정된 건의 확정 해제  → 실행 결과 1,124건
UPDATE as_receipts SET schedule_confirmed = 0
 WHERE schedule_confirmed = 1
   AND TRIM(COALESCE(visit_scheduled_date,'')) = '';

-- ========== B.4 트리거 제약 (S-6 ~ S-8) ==========
-- S-6: 일정확정에는 예정업무일이 반드시 있어야 한다
DROP TRIGGER IF EXISTS trg_as_sched_confirm_ins;
CREATE TRIGGER trg_as_sched_confirm_ins BEFORE INSERT ON as_receipts
FOR EACH ROW WHEN NEW.schedule_confirmed = 1
                 AND TRIM(COALESCE(NEW.visit_scheduled_date,'')) = ''
BEGIN SELECT RAISE(ABORT, '일정확정에는 예정업무일이 필요합니다'); END;

DROP TRIGGER IF EXISTS trg_as_sched_confirm_upd;
CREATE TRIGGER trg_as_sched_confirm_upd BEFORE UPDATE ON as_receipts
FOR EACH ROW WHEN NEW.schedule_confirmed = 1
                 AND TRIM(COALESCE(NEW.visit_scheduled_date,'')) = ''
BEGIN SELECT RAISE(ABORT, '일정확정에는 예정업무일이 필요합니다'); END;

-- S-7: 조치 소요시간은 0보다 커야 한다
DROP TRIGGER IF EXISTS trg_proc_time_spent_ins;
CREATE TRIGGER trg_proc_time_spent_ins BEFORE INSERT ON as_processes
FOR EACH ROW WHEN COALESCE(NEW.time_spent, 0) <= 0
BEGIN SELECT RAISE(ABORT, '소요시간(분)은 0보다 커야 합니다'); END;

-- S-8: 근무구분은 office 또는 field
DROP TRIGGER IF EXISTS trg_as_work_place_upd;
CREATE TRIGGER trg_as_work_place_upd BEFORE UPDATE ON as_receipts
FOR EACH ROW WHEN TRIM(COALESCE(NEW.work_place,'')) <> ''
                 AND NEW.work_place NOT IN ('office','field')
BEGIN SELECT RAISE(ABORT, '근무구분은 office 또는 field 여야 합니다'); END;

-- ========== B.3 정기점검 방문 중복 방지 (S-5) ==========
CREATE UNIQUE INDEX IF NOT EXISTS ux_mnt_visit_unique
  ON maintenance_visits(plan_id, visit_date, customer_id, COALESCE(product_type,''));
