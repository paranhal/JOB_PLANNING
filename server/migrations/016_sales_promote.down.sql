-- 016 롤백. 연결 컬럼만 뗀다. 영업 사업·활동 로그·이미 만든 work_projects 행은 지우지 않는다.

DROP INDEX IF EXISTS idx_work_projects_sales;

ALTER TABLE work_projects DROP COLUMN sales_project_id;

DELETE FROM id_sequences WHERE seq_key = '__meta:sales_promote_v1';
