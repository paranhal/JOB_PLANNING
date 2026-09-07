-- 기획서 §34.3.3 원인분류 3단계 시드 v2 (2026-09-02 엑셀).
-- 실제 앱은 InitDB 시 repository.applyASCauseCategoriesV2 가 AS_원인분류_시드.sql 내용을 실행한다.
-- 라벨만 갱신하고 코드는 유지한다. 신규 코드는 server.patch 하나.

ALTER TABLE as_cause_categories ADD COLUMN cause_type_map TEXT NOT NULL DEFAULT '';
ALTER TABLE as_cause_categories ADD COLUMN is_fault INTEGER NOT NULL DEFAULT 1;
