-- 74-B D-66. AS 기관 직접입력용 임시(미확정) 고객.

ALTER TABLE customers ADD COLUMN is_provisional INTEGER NOT NULL DEFAULT 0;
ALTER TABLE customers ADD COLUMN provisional_by TEXT NOT NULL DEFAULT '';
ALTER TABLE customers ADD COLUMN provisional_at TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_customers_provisional ON customers(is_provisional);
