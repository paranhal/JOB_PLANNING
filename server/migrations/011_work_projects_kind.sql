-- 기획서 §22.1.1 ① · ⑪단계 1
-- work_projects 에 사업 유형을 둔다. 기존 건은 전부 유지보수다.
-- 실제 앱은 InitDB 시 repository.applyProjectKind 가 동일 SQL을 실행한다.

ALTER TABLE work_projects ADD COLUMN project_kind TEXT NOT NULL DEFAULT 'maintenance';

UPDATE work_projects
   SET project_kind = 'maintenance'
 WHERE TRIM(COALESCE(project_kind,'')) = ''
    OR project_id LIKE 'WPSEED%';

INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order) VALUES
  ('PK001','project_kind','maintenance','유지보수',1),
  ('PK002','project_kind','build','신규구축',2),
  ('PK003','project_kind','supply','장비납품',3),
  ('PK004','project_kind','consumable','소모품납품',4),
  ('PK005','project_kind','other','기타',5),
  ('WPK001','weekly_ref_project_kind','build','정보시스템_구축',1),
  ('WPK002','weekly_ref_project_kind','build','1_CCTV_설치',2),
  ('WPK003','weekly_ref_project_kind','build','2_통신_공사',3),
  ('WPK004','weekly_ref_project_kind','build','3_무선_인프라_구축',4),
  ('WPK005','weekly_ref_project_kind','maintenance','4_인프라_유지_관리',5),
  ('WPK006','weekly_ref_project_kind','other','기타_활동',6),
  ('WPK007','weekly_ref_project_kind','supply','기타_활동',7),
  ('WPK008','weekly_ref_project_kind','consumable','기타_활동',8);
