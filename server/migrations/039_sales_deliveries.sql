-- 039 납품·설치자산 연결 (§36.10 · §36.10.1).
-- 실제 적용은 applySalesOrders → applySalesDeliveries.

CREATE TABLE IF NOT EXISTS sales_deliveries (
  delivery_id    TEXT PRIMARY KEY,
  order_id       TEXT NOT NULL,
  line_id        TEXT NOT NULL DEFAULT '',
  delivery_date  TEXT NOT NULL,
  qty            REAL NOT NULL DEFAULT 0,
  place          TEXT NOT NULL DEFAULT '',
  receiver_name  TEXT NOT NULL DEFAULT '',
  installed_by   TEXT NOT NULL DEFAULT '',
  note           TEXT NOT NULL DEFAULT '',
  created_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (order_id) REFERENCES sales_orders(order_id)
);

CREATE INDEX IF NOT EXISTS idx_sales_deliveries_order ON sales_deliveries(order_id);
CREATE INDEX IF NOT EXISTS idx_sales_deliveries_line ON sales_deliveries(line_id);

-- assets.sales_order_id 는 applySalesDeliveries 에서 ALTER 로 붙인다.
