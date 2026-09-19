-- 054 영업 핵심 전략 메모. 한 줄 = 한 행. §46.4
CREATE TABLE IF NOT EXISTS sales_memos (
  memo_id     TEXT PRIMARY KEY,
  sales_id    TEXT NOT NULL,
  content     TEXT NOT NULL,
  sort_order  INTEGER NOT NULL DEFAULT 0,
  author_id   TEXT NOT NULL DEFAULT '',
  author_name TEXT NOT NULL DEFAULT '',
  created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (sales_id) REFERENCES sales_projects(sales_id)
);
CREATE INDEX IF NOT EXISTS idx_sales_memos_sales ON sales_memos(sales_id, sort_order);
