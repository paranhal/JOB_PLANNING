-- 95-126 빠른 견적: 수신 연락처 스냅샷, 다운로드·발송 시각, 절사 위치, 등록 경로
ALTER TABLE sales_quotes ADD COLUMN attn_phone TEXT NOT NULL DEFAULT '';
ALTER TABLE sales_quotes ADD COLUMN attn_email TEXT NOT NULL DEFAULT '';
ALTER TABLE sales_quotes ADD COLUMN downloaded_at TEXT NOT NULL DEFAULT '';
ALTER TABLE sales_quotes ADD COLUMN sent_at TEXT NOT NULL DEFAULT '';
ALTER TABLE sales_quotes ADD COLUMN cut_scope TEXT NOT NULL DEFAULT 'total';
ALTER TABLE sales_projects ADD COLUMN created_via TEXT NOT NULL DEFAULT '';
