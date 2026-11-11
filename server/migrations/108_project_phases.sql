-- 95-120P 사업 단계. 기존 사업·업무 행은 바꾸지 않는다. ID는 TEXT.

CREATE TABLE IF NOT EXISTS project_phases (
  phase_id     TEXT PRIMARY KEY,
  project_id   TEXT NOT NULL,
  org_id       TEXT NOT NULL,
  seq          INTEGER NOT NULL DEFAULT 0,
  name         TEXT NOT NULL,
  plan_start   TEXT NOT NULL DEFAULT '',
  plan_end     TEXT NOT NULL DEFAULT '',
  actual_start TEXT NOT NULL DEFAULT '',
  actual_end   TEXT NOT NULL DEFAULT '',
  status       TEXT NOT NULL DEFAULT 'planned',
  note         TEXT NOT NULL DEFAULT '',
  is_active    INTEGER NOT NULL DEFAULT 1,
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL,
  UNIQUE(project_id, name)
);
CREATE INDEX IF NOT EXISTS idx_project_phases_project ON project_phases(project_id, seq);

-- 기본 단계 제안. 코드 관리에서 고칠 수 있다. 사업 행은 만들지 않는다.
INSERT OR IGNORE INTO codes(code_id, code_group, code_value, code_name, sort_order, is_active) VALUES
 ('PHC-B01','phase_tpl_build','착수','착수',10,1),
 ('PHC-B02','phase_tpl_build','분석','분석',20,1),
 ('PHC-B03','phase_tpl_build','설계','설계',30,1),
 ('PHC-B04','phase_tpl_build','구축 및 설치','구축 및 설치',40,1),
 ('PHC-B05','phase_tpl_build','시험 및 검수','시험 및 검수',50,1),
 ('PHC-B06','phase_tpl_build','인수 및 종료','인수 및 종료',60,1),
 ('PHC-D01','phase_tpl_develop','착수','착수',10,1),
 ('PHC-D02','phase_tpl_develop','요구분석','요구분석',20,1),
 ('PHC-D03','phase_tpl_develop','설계','설계',30,1),
 ('PHC-D04','phase_tpl_develop','개발','개발',40,1),
 ('PHC-D05','phase_tpl_develop','시험','시험',50,1),
 ('PHC-D06','phase_tpl_develop','인수','인수',60,1),
 ('PHC-C01','phase_tpl_construction','착공','착공',10,1),
 ('PHC-C02','phase_tpl_construction','시공','시공',20,1),
 ('PHC-C03','phase_tpl_construction','준공 검사','준공 검사',30,1),
 ('PHC-C04','phase_tpl_construction','인수','인수',40,1);
