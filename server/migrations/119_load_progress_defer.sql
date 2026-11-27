-- 95-226: 하부 작업 부하량(%) · 일자별 부분 완료 · 수동 진행률 · 연기(대기 업무) · 완료일 보존(95-223 #4).
-- 하부 업무(phase_packages)·하위 업무(work_tasks.parent_task_id)의 부하량. 비어 있으면 균등 분할(계산으로 처리), 사이트가 있는 대상은 쓰지 않는다.
ALTER TABLE phase_packages ADD COLUMN load_pct REAL;
ALTER TABLE work_tasks ADD COLUMN load_pct REAL;
-- 수동 진행률(자동값과 병기, 수동 우선 표시). 변경 이력은 data_change_logs.
ALTER TABLE work_tasks ADD COLUMN progress_manual REAL;
-- 남은 업무/사이트 수(저장해 두었다가 다음 날 이어서 표시)
ALTER TABLE work_tasks ADD COLUMN remain_count INTEGER;
-- 연기: 날짜를 비우고 status 는 waiting 을 유지한다. 미정 사유는 시스템이 「연기(대기)」로 기록한다(v2.0 미정+사유).
ALTER TABLE work_tasks ADD COLUMN no_date_reason TEXT;
ALTER TABLE work_tasks ADD COLUMN deferred_at TEXT;
ALTER TABLE work_tasks ADD COLUMN defer_from_date TEXT;
-- 대기에서 오늘 업무로 가져온 건(그날 끝나도 미완료이면 다시 대기로 복귀)
ALTER TABLE work_tasks ADD COLUMN from_deferred INTEGER NOT NULL DEFAULT 0;
-- 완료일 보존(95-223 #4): 진행중으로 되돌릴 때 이전 완료일을 보관하고, 다시 완료하면 기본으로 쓴다.
ALTER TABLE work_tasks ADD COLUMN prev_complete_date TEXT;

-- 한 업무 안의 일자별 부하량·부분 완료(여러 날에 걸친 단일 업무). 날짜별 형제 업무 구조에서는 쓰지 않는다.
CREATE TABLE IF NOT EXISTS work_task_day_progress (
  task_id    TEXT NOT NULL,
  work_date  TEXT NOT NULL,
  load_pct   REAL,                       -- 그날 몫(%). 비면 기간 균등 분할
  done_pct   REAL NOT NULL DEFAULT 0,    -- 그날 몫 중 끝낸 비율(0~100)
  note       TEXT NOT NULL DEFAULT '',
  done_by    TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (task_id, work_date)
);
CREATE INDEX IF NOT EXISTS idx_wtdp_task ON work_task_day_progress(task_id);
