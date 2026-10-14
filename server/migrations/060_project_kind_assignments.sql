-- 72-A2 D-53·D-54. 사업 유형(project_kind)과 담당자 4역할.
-- 컬럼 이름은 011에서 쓰던 project_kind. 값 집합은 공사/구축/개발/유지보수로 바꾼다.
-- 예전 코드값(supply·consumable·other, 라벨 신규구축)은 시드하지 않는다.

ALTER TABLE work_projects ADD COLUMN project_kind TEXT NOT NULL DEFAULT 'maintenance';

UPDATE work_projects
   SET project_kind = 'maintenance'
 WHERE TRIM(COALESCE(project_kind,'')) = '';

CREATE TABLE IF NOT EXISTS project_assignments (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id TEXT NOT NULL,
  user_id    TEXT NOT NULL,
  duty       TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  created_by TEXT,
  UNIQUE (project_id, user_id, duty)
);
CREATE INDEX IF NOT EXISTS idx_pa_project ON project_assignments(project_id);
CREATE INDEX IF NOT EXISTS idx_pa_user ON project_assignments(user_id);

INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order) VALUES
  ('PKC01','project_kind','construction','공사',10),
  ('PKC02','project_kind','build','구축',20),
  ('PKC03','project_kind','dev','개발',30),
  ('PKC04','project_kind','maintenance','유지보수',40);

UPDATE codes SET code_name='구축'
 WHERE code_group='project_kind' AND code_value='build' AND code_name='신규구축';
