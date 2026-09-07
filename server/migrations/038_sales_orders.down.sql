-- 038 롤백. 수주·발주 표를 뗀다. 037 견적·노임단가는 남긴다.

DROP INDEX IF EXISTS idx_sales_purchases_line;
DROP INDEX IF EXISTS idx_sales_purchases_order;
DROP TABLE IF EXISTS sales_purchases;
DROP INDEX IF EXISTS idx_sales_order_lines_order;
DROP TABLE IF EXISTS sales_order_lines;
DROP INDEX IF EXISTS idx_sales_orders_status;
DROP INDEX IF EXISTS idx_sales_orders_sales;
DROP INDEX IF EXISTS idx_sales_orders_quote;
DROP INDEX IF EXISTS idx_sales_orders_no;
DROP TABLE IF EXISTS sales_orders;
