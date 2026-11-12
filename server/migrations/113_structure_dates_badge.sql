-- 95-179: 하부 업무 일자·배지, 마감 변경(연장) 이력. 기존 행 불변(빈 값).
-- 하부 업무(phase_packages): 예정·마감 일자(선택), 실제 시작일(수동 입력값. 비면 연결 작업의 첫 완료일을 자동으로 쓴다), 배지(8자 이내, 선택)
ALTER TABLE phase_packages ADD COLUMN plan_start TEXT NOT NULL DEFAULT '';
ALTER TABLE phase_packages ADD COLUMN plan_end TEXT NOT NULL DEFAULT '';
ALTER TABLE phase_packages ADD COLUMN actual_start TEXT NOT NULL DEFAULT '';
ALTER TABLE phase_packages ADD COLUMN badge TEXT NOT NULL DEFAULT '';

-- 마감일 변경 이력: 연장(뒤로 미룸)은 사유 필수, 앞당김·해제는 이력만(사유 선택). 변경한 날짜·아이디·사유를 남긴다.
CREATE TABLE IF NOT EXISTS structure_date_changes (
  change_id   TEXT PRIMARY KEY,
  project_id  TEXT NOT NULL,
  org_id      TEXT NOT NULL DEFAULT '',
  level       TEXT NOT NULL,              -- phase | package
  target_id   TEXT NOT NULL,
  target_name TEXT NOT NULL DEFAULT '',
  field       TEXT NOT NULL DEFAULT 'plan_end',
  kind        TEXT NOT NULL,              -- extend | shorten | clear
  old_value   TEXT NOT NULL DEFAULT '',
  new_value   TEXT NOT NULL DEFAULT '',
  reason      TEXT NOT NULL DEFAULT '',
  changed_by  TEXT NOT NULL DEFAULT '',
  changed_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_structure_date_changes_project ON structure_date_changes(project_id, changed_at);
