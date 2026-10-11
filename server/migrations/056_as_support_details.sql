-- 056 AS 지원업무 1:1 상세. as_work_items.work_kind='support' (개정안 §75)
CREATE TABLE IF NOT EXISTS as_support_details (
  work_id       TEXT PRIMARY KEY REFERENCES as_work_items(work_id),
  support_type  TEXT NOT NULL,
  vendor_name   TEXT,
  tracking_no   TEXT,
  action_date   TEXT,
  due_date      TEXT,
  asset_id      TEXT,
  memo          TEXT
);
CREATE INDEX IF NOT EXISTS idx_as_support_details_type ON as_support_details(support_type);
CREATE INDEX IF NOT EXISTS idx_as_support_details_asset ON as_support_details(asset_id);
