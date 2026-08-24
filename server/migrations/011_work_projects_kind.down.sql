-- 011 롤백. 사업 유형 컬럼과 코드 매핑을 제거한다.
-- SQLite 3.35+ 의 DROP COLUMN 을 쓴다.

DELETE FROM codes WHERE code_group IN ('project_kind','weekly_ref_project_kind');
ALTER TABLE work_projects DROP COLUMN project_kind;
