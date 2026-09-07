-- 039 롤백. 납품 표를 뗀다. assets.sales_order_id 는 남겨 둔다.

DROP INDEX IF EXISTS idx_sales_deliveries_line;
DROP INDEX IF EXISTS idx_sales_deliveries_order;
DROP TABLE IF EXISTS sales_deliveries;
