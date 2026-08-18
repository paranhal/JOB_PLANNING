-- 기획서 §16.6.9 · §15
-- as_receipts 에 사업 귀속(project_id)을 저장한다.
-- 조회 시점에 project_scope_rules 를 다시 풀지 않는다. 규칙이 바뀌어도
-- 그 시점의 귀속을 보존한다.
-- 실제 앱은 InitDB 시 repository.applyASReceiptsProjectID 가 동일 SQL을 실행한 뒤
-- maintenance_visits · as_receipts 의 빈 값을 ResolveProjectID 로 백필한다.

ALTER TABLE as_receipts ADD COLUMN project_id TEXT;
CREATE INDEX IF NOT EXISTS idx_as_receipts_project ON as_receipts(project_id);
