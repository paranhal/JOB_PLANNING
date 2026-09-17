-- §4.7 방문일(visit_date) · 처리유형 미정+사유.
-- 착수일시(start_datetime)는 그대로 둔다. 방문 지표만 visit_date 를 쓴다.
-- 실제 앱은 InitDB 시 repository.applyAS47VisitDate 가 동일 내용을 실행한다.

ALTER TABLE as_receipts ADD COLUMN visit_date TEXT;
ALTER TABLE as_receipts ADD COLUMN process_type_reason TEXT;

INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active)
VALUES ('PRT006','process_type','undetermined','미정',6,1);
